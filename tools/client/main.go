package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"sync"
	"time"
)

type config struct {
	host          string
	port          int
	clients       int
	balance       int
	baseCost      int
	minBPM        int
	maxBPM        int
	overbidLimit  int
	interDelay    time.Duration
	roundDuration time.Duration
	trialDuration time.Duration
	seed          int64
}

func main() {
	cfg := parseFlags()
	rand.Seed(cfg.seed)

	addr := fmt.Sprintf("%s:%d", cfg.host, cfg.port)
	deadline := time.Now().Add(cfg.trialDuration)
	ctxTimeout := cfg.trialDuration + cfg.roundDuration + time.Second

	var wg sync.WaitGroup
	logCh := make(chan string, cfg.clients*2)
	stopLog := make(chan struct{})

	go logger(logCh, stopLog)

	for i := 0; i < cfg.clients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runClient(id, addr, cfg, deadline, logCh)
		}(i)
	}

	// Wait for clients with a safety timeout in case something wedges.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(ctxTimeout):
		logCh <- "[runner] timeout waiting for clients to finish"
	}

	close(logCh)
	<-stopLog
}

func parseFlags() config {
	cfg := config{}

	flag.StringVar(&cfg.host, "host", "127.0.0.1", "server host")
	flag.IntVar(&cfg.port, "port", 8080, "server port")
	flag.IntVar(&cfg.clients, "clients", 10, "number of concurrent simulated validators")
	flag.IntVar(&cfg.balance, "balance", 1000, "initial token balance per validator")
	flag.IntVar(&cfg.baseCost, "base-cost", 15, "base bidding cost used to derive random bids")
	flag.IntVar(&cfg.minBPM, "bpm-min", 60, "minimum BPM value sent to the server")
	flag.IntVar(&cfg.maxBPM, "bpm-max", 80, "maximum BPM value sent to the server")
	flag.IntVar(&cfg.overbidLimit, "overbid-limit", 100, "upper bound for per-client overbid percentage")

	var interDelaySec float64
	var roundDurationSec float64
	var trialDurationSec float64
	flag.Float64Var(&interDelaySec, "inter-delay", 1.0, "delay in seconds between BPM and bid submissions")
	flag.Float64Var(&roundDurationSec, "round", 60.0, "seconds between successive bids from the same validator")
	flag.Float64Var(&trialDurationSec, "duration", 300.0, "total experiment duration in seconds")
	flag.Int64Var(&cfg.seed, "seed", time.Now().UnixNano(), "seed for the shared RNG")

	flag.Parse()

	cfg.interDelay = time.Duration(interDelaySec * float64(time.Second))
	cfg.roundDuration = time.Duration(roundDurationSec * float64(time.Second))
	cfg.trialDuration = time.Duration(trialDurationSec * float64(time.Second))

	if cfg.minBPM >= cfg.maxBPM {
		cfg.maxBPM = cfg.minBPM + 1
	}

	if cfg.baseCost <= 0 {
		cfg.baseCost = 1
	}

	if cfg.overbidLimit <= 0 {
		cfg.overbidLimit = 100
	}

	return cfg
}

func runClient(id int, addr string, cfg config, deadline time.Time, logCh chan<- string) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		logCh <- fmt.Sprintf("[client-%d] dial error: %v", id, err)
		return
	}
	defer conn.Close()

	if err := conn.SetDeadline(deadline.Add(cfg.interDelay * 2)); err != nil {
		logCh <- fmt.Sprintf("[client-%d] set deadline error: %v", id, err)
	}

	go io.Copy(ioutil.Discard, conn) // drain server announcements to avoid blocking

	writer := bufio.NewWriter(conn)
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)*7919))

	initialBalance := cfg.balance
	if initialBalance < 1 {
		initialBalance = 1
	}

	if err := sendLine(writer, initialBalance); err != nil {
		logCh <- fmt.Sprintf("[client-%d] send balance error: %v", id, err)
		return
	}

	// Randomize the initial scheduling slightly so that clients stagger naturally.
	initialSleep := time.Duration(rng.Intn(500)) * time.Millisecond
	time.Sleep(initialSleep)

	overbidPercent := rng.Intn(cfg.overbidLimit) + 1
	overbidActive := rng.Intn(cfg.overbidLimit) < overbidPercent

	doRound := func() error {
		bpm := cfg.minBPM + rng.Intn(cfg.maxBPM-cfg.minBPM+1)
		if err := sendLine(writer, bpm); err != nil {
			return fmt.Errorf("send BPM: %w", err)
		}
		time.Sleep(cfg.interDelay)

		bid := rng.Intn(cfg.baseCost) + 1
		if overbidActive {
			overbidCheck := rng.Intn(cfg.overbidLimit) + 1
			if overbidCheck <= overbidPercent {
				bid = cfg.baseCost + rng.Intn(cfg.baseCost) + 1
			}
		}

		if err := sendLine(writer, bid); err != nil {
			return fmt.Errorf("send bid: %w", err)
		}

		return nil
	}

	if err := doRound(); err != nil {
		logCh <- fmt.Sprintf("[client-%d] initial round error: %v", id, err)
		return
	}

	deadlineTimer := time.NewTimer(time.Until(deadline))
	defer deadlineTimer.Stop()

	intervalTicker := time.NewTicker(cfg.roundDuration)
	defer intervalTicker.Stop()

	for {
		select {
		case <-deadlineTimer.C:
			logCh <- fmt.Sprintf("[client-%d] completed", id)
			return
		case <-intervalTicker.C:
			if err := doRound(); err != nil {
				logCh <- fmt.Sprintf("[client-%d] round error: %v", id, err)
				return
			}
		}
	}
}

func sendLine(w *bufio.Writer, value int) error {
	if _, err := fmt.Fprintf(w, "%d\n", value); err != nil {
		return err
	}
	return w.Flush()
}

func logger(ch <-chan string, done chan<- struct{}) {
	for msg := range ch {
		fmt.Println(msg)
	}
	close(done)
}
