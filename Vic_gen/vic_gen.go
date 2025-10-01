package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
	"github.com/tealeg/xlsx"
)

type Block struct {
	Index     int
	Timestamp string
	BPM       int
	Hash      string
	PrevHash  string
	Validator string
	Proposer  string
	Transfer  int
}

var Blockchain []Block
var tempBlocks []Block

var candidateBlocks = make(chan Block)

var announcements = make(chan string)

var mutex = &sync.Mutex{}

type Node struct {
	Address string
	Balance int
	Bid     int
}

type BidItem struct {
	NodeAddress string
	Bid         int
}

var validators = make(map[string]*Node)
var bids = make([]BidItem, 0)

var miningCost int

const burnRate = 0.05

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	t := time.Now()
	genesisBlock := Block{0, t.String(), 0, calculateBlockHash(Block{}), "", "", "", 0}
	spew.Dump(genesisBlock)
	Blockchain = append(Blockchain, genesisBlock)

	tcpPort := os.Getenv("PORT")

	server, err := net.Listen("tcp", ":"+tcpPort)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("TCP Server Listening on port :", tcpPort)
	defer server.Close()

	go func() {
		for candidate := range candidateBlocks {
			mutex.Lock()
			tempBlocks = append(tempBlocks, candidate)
			mutex.Unlock()
		}
	}()

	go func() {
		for {
			pickWinner()
			//time.Sleep(30 * time.Second)
			printGiniCoefficient()
			exportBlockchainToExcel()
		}
	}()

	for {
		conn, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	address := ""

	go func() {
		for {
			msg := <-announcements
			io.WriteString(conn, msg)

			if address != "" {
				balanceMsg := "Your current balance: " + strconv.Itoa(validators[address].Balance) + "\n"
				io.WriteString(conn, balanceMsg)
			}
		}
	}()

	io.WriteString(conn, "Enter token balance:")
	scanBalance := bufio.NewScanner(conn)
	scanBalance.Scan()
	balance, err := strconv.Atoi(scanBalance.Text())
	if err != nil {
		log.Printf("%v not a number: %v", scanBalance.Text(), err)
		return
	}

	t := time.Now()
	address = calculateHash(t.String())
	node := &Node{Address: address, Balance: balance, Bid: 0}

	validators[address] = node

	io.WriteString(conn, "\nEnter a new BPM:")

	scanBPM := bufio.NewScanner(conn)

	go func() {
		for {
			for scanBPM.Scan() {
				bpm, err := strconv.Atoi(scanBPM.Text())
				if err != nil {
					log.Printf("%v not a number: %v", scanBPM.Text(), err)
					conn.Close()
				}

				io.WriteString(conn, "\nSubmit your bid:")
				scanBid := bufio.NewScanner(conn)
				scanBid.Scan()
				bid, err := strconv.Atoi(scanBid.Text())
				if err != nil {
					log.Printf("%v not a number: %v", scanBid.Text(), err)
					return
				}

				if node.Balance >= bid {
					node.Balance -= bid
					node.Bid = bid
					mutex.Lock()
					validators[address] = node
					mutex.Unlock()
					bids = append(bids, BidItem{NodeAddress: address, Bid: bid})

					// only generate a block when a valid bid is received
					mutex.Lock()
					oldLastIndex := Blockchain[len(Blockchain)-1]
					mutex.Unlock()

					newBlock := generateBlock(oldLastIndex, bpm, address)

					if isBlockValid(newBlock, oldLastIndex) {
						candidateBlocks <- newBlock
					}
				}
				io.WriteString(conn, "\nBid submitted, waiting for auction result.")
				io.WriteString(conn, "\nEnter a new BPM:")
			}
		}
	}()

	for {
		time.Sleep(58 * time.Second)
		mutex.Lock()
		output, err := json.Marshal(Blockchain)
		mutex.Unlock()
		if err != nil {
			log.Fatal(err)
		}
		io.WriteString(conn, string(output)+"\n")
	}
}

// Function using Vickrey Auction mechanism
func pickWinner() {
	time.Sleep(60 * time.Second)

	mutex.Lock()
	blockCandidates := append([]Block(nil), tempBlocks...)
	roundBids := append([]BidItem(nil), bids...)
	mutex.Unlock()

	if len(blockCandidates) == 0 || len(roundBids) == 0 {
		mutex.Lock()
		tempBlocks = []Block{}
		bids = []BidItem{}
		updateMiningCost(0)
		mutex.Unlock()
		return
	}

	weights := make(map[string]int)
	for _, bidItem := range roundBids {
		if bidItem.Bid <= 0 {
			continue
		}
		weights[bidItem.NodeAddress] += bidItem.Bid
	}

	if len(weights) == 0 {
		mutex.Lock()
		tempBlocks = []Block{}
		bids = []BidItem{}
		updateMiningCost(0)
		mutex.Unlock()
		return
	}

	winner := weightedWinner(weights)
	if winner == "" {
		mutex.Lock()
		tempBlocks = []Block{}
		bids = []BidItem{}
		updateMiningCost(0)
		mutex.Unlock()
		return
	}

	secondPrice := secondHighestBid(weights, winner)
	winnerBid := weights[winner]
	if secondPrice > winnerBid {
		secondPrice = winnerBid
	}

	selectedBlock := selectBlockForWinner(blockCandidates, winner)

	mutex.Lock()
	for addr := range weights {
		if node, ok := validators[addr]; ok {
			node.Balance += node.Bid
			node.Bid = 0
		}
	}

	priceCharged := secondPrice
	if node, ok := validators[winner]; ok {
		if priceCharged > node.Balance {
			priceCharged = node.Balance
		}
		node.Balance -= priceCharged
	}

	selectedBlock.Validator = winner
	selectedBlock.Transfer = priceCharged
	Blockchain = append(Blockchain, selectedBlock)

	tempBlocks = []Block{}
	bids = []BidItem{}
	updateMiningCost(priceCharged)
	announcementCount := len(validators)
	mutex.Unlock()

	for i := 0; i < announcementCount; i++ {
		announcements <- "\nwinning validator: " + winner + "\n"
	}
}

func isBlockValid(newBlock, oldBlock Block) bool {
	if oldBlock.Index+1 != newBlock.Index {
		return false
	}

	if oldBlock.Hash != newBlock.PrevHash {
		return false
	}

	if calculateBlockHash(newBlock) != newBlock.Hash {
		return false
	}

	return true
}

func calculateBlockHash(block Block) string {
	record := string(block.Index) + block.Timestamp + string(block.BPM) + block.PrevHash
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func generateBlock(oldBlock Block, BPM int, address string) Block {
	var newBlock Block

	t := time.Now()

	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateBlockHash(newBlock)
	newBlock.Proposer = address

	return newBlock
}

func calculateHash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func updateMiningCost(cost int) {
	miningCost = int(float64(cost) * burnRate)
}

func weightedWinner(weights map[string]int) string {
	keys := make([]string, 0, len(weights))
	total := 0
	for addr, weight := range weights {
		if weight <= 0 {
			continue
		}
		keys = append(keys, addr)
		total += weight
	}

	if total == 0 || len(keys) == 0 {
		return ""
	}

	sort.Strings(keys)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	threshold := rng.Intn(total)
	cumulative := 0
	for _, addr := range keys {
		cumulative += weights[addr]
		if threshold < cumulative {
			return addr
		}
	}

	return keys[len(keys)-1]
}

func secondHighestBid(weights map[string]int, winner string) int {
	ordered := make([]BidItem, 0, len(weights))
	for addr, weight := range weights {
		if weight <= 0 {
			continue
		}
		ordered = append(ordered, BidItem{NodeAddress: addr, Bid: weight})
	}

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Bid > ordered[j].Bid
	})

	for _, item := range ordered {
		if item.NodeAddress == winner {
			continue
		}
		return item.Bid
	}

	return 0
}

func selectBlockForWinner(blocks []Block, winner string) Block {
	if len(blocks) == 0 {
		return Block{}
	}

	for _, block := range blocks {
		if block.Proposer == winner {
			return block
		}
	}

	return blocks[0]
}

func printGiniCoefficient() {
	balances := make([]int, 0)
	for _, node := range validators {
		balances = append(balances, node.Balance)
	}
	gini := giniCoefficient(balances)
	fmt.Println("Gini Coefficient: ", gini)
}

func giniCoefficient(incomes []int) float64 {
	n := float64(len(incomes))
	if n == 0 {
		return 0
	}

	mean := 0.0
	sumOfAbsoluteDifferences := 0.0

	for _, income := range incomes {
		mean += float64(income)
		for _, otherIncome := range incomes {
			sumOfAbsoluteDifferences += math.Abs(float64(income - otherIncome))
		}
	}

	mean /= n

	return sumOfAbsoluteDifferences / (2 * n * n * mean)
}

func exportBlockchainToExcel() {
	file := xlsx.NewFile()
	sheet, err := file.AddSheet("Blockchain")
	if err != nil {
		log.Fatalf("cannot add sheet: %v", err)
	}

	row := sheet.AddRow()
	row.AddCell().Value = "Index"
	row.AddCell().Value = "Timestamp"
	row.AddCell().Value = "BPM"
	row.AddCell().Value = "Hash"
	row.AddCell().Value = "PrevHash"
	row.AddCell().Value = "Validator"
	row.AddCell().Value = "Proposer"
	row.AddCell().Value = "Transfer"

	for _, block := range Blockchain {
		row := sheet.AddRow()
		row.AddCell().Value = strconv.Itoa(block.Index)
		row.AddCell().Value = block.Timestamp
		row.AddCell().Value = strconv.Itoa(block.BPM)
		row.AddCell().Value = block.Hash
		row.AddCell().Value = block.PrevHash
		row.AddCell().Value = block.Validator
		row.AddCell().Value = block.Proposer
		row.AddCell().Value = strconv.Itoa(block.Transfer)
	}

	if err := file.Save("Blockchain.xlsx"); err != nil {
		log.Fatalf("cannot save file: %v", err)
	}
}
