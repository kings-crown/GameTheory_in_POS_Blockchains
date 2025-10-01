package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
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
	mutex.Lock()
	validators[address] = node
	mutex.Unlock()

	io.WriteString(conn, "\nEnter a new BPM:")

	scanBPM := bufio.NewScanner(conn)
	for scanBPM.Scan() {
		bpm, err := strconv.Atoi(scanBPM.Text())
		if err != nil {
			log.Printf("%v not a number: %v", scanBPM.Text(), err)
			break
		}

		io.WriteString(conn, "\nSubmit your bid:")
		scanBid := bufio.NewScanner(conn)
		scanBid.Scan()
		bid, err := strconv.Atoi(scanBid.Text())
		if err != nil {
			log.Printf("%v not a number: %v", scanBid.Text(), err)
			break
		}

		node := validators[address]
		if node.Balance < bid {
			log.Println("Bid is more than your balance")
			continue
		}

		node.Balance -= bid
		node.Bid = bid

		newBlock := generateBlock(Blockchain[len(Blockchain)-1], bpm, address, bid)
		if isBlockValid(newBlock, Blockchain[len(Blockchain)-1]) {
			candidateBlocks <- newBlock
		}
	}
}

func calculateHash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

func calculateBlockHash(block Block) string {
	record := string(block.Index) + block.Timestamp + string(block.BPM) + block.PrevHash
	return calculateHash(record)
}

func generateBlock(oldBlock Block, BPM int, address string, bid int) Block {
	var newBlock Block

	t := time.Now()

	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateBlockHash(newBlock)
	newBlock.Validator = address
	newBlock.Proposer = ""
	newBlock.Transfer = 0

	return newBlock
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

func pickWinner() {
	time.Sleep(60 * time.Second)
	mutex.Lock()
	temp := tempBlocks
	mutex.Unlock()

	lotteryPool := make([]string, 0)

	if len(temp) > 0 {
	OUTER:
		for _, block := range temp {
			for _, node := range lotteryPool {
				if block.Validator == node {
					continue OUTER
				}
			}

			mutex.Lock()
			set := validators[block.Validator]
			mutex.Unlock()

			k := set.Balance

			for i := 0; i < k; i++ {
				lotteryPool = append(lotteryPool, block.Validator)
			}
		}

		s := rand.NewSource(time.Now().Unix())
		r := rand.New(s)
		lotteryWinner := lotteryPool[r.Intn(len(lotteryPool))]

		for _, block := range temp {
			if block.Validator == lotteryWinner {
				mutex.Lock()
				Blockchain = append(Blockchain, block)
				mutex.Unlock()
				for _ = range validators {
					announcements <- "\nwinning validator: " + lotteryWinner + "\n"
				}
				break
			}
		}
	}

	mutex.Lock()
	tempBlocks = []Block{}
	mutex.Unlock()
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
	if len(incomes) == 0 {
		return 0
	}

	sort.Ints(incomes)
	var sumOfAbsoluteDifferences float64 = 0
	subSum := 0
	for i, income := range incomes {
		sumOfAbsoluteDifferences += float64(income * (2*i - len(incomes) + 1))
		subSum += income
	}

	return sumOfAbsoluteDifferences / float64(subSum*len(incomes))
}

func exportBlockchainToExcel() {
	// Create new file
	file := xlsx.NewFile()
	sheet, _ := file.AddSheet("Blockchain")

	// Create header
	row := sheet.AddRow()
	row.AddCell().Value = "Index"
	row.AddCell().Value = "Timestamp"
	row.AddCell().Value = "BPM"
	row.AddCell().Value = "Hash"
	row.AddCell().Value = "PrevHash"
	row.AddCell().Value = "Validator"
	row.AddCell().Value = "Proposer"
	row.AddCell().Value = "Transfer"

	// Add data
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

	// Save to blockchain.xlsx
	err := file.Save("blockchain.xlsx")
	if err != nil {
		fmt.Printf("Error saving to excel: %v\n", err)
	} else {
		fmt.Println("Blockchain successfully exported to blockchain.xlsx")
	}
}

func handleWrite(conn net.Conn, text string) {
	_, err := io.WriteString(conn, text+"\n")
	if err != nil {
		log.Fatal(err)
	}
}
