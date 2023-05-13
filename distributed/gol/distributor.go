package gol

import (
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	ioKeyPress <- chan rune
}



func checkerr(e error, lineno int){
	if e != nil {
		fmt.Fprintf(os.Stderr, "Error occured: ", e, " \nline: ", lineno)
		os.Exit(2)
	}
}

// Sends an alive cells count event every 2 seconds
func reportAlive(d distributorChannels, done chan bool, conn *rpc.Client, mutex *sync.Mutex) {
	ticker := time.NewTicker(2 * time.Second)
	flag := false
	for !flag {
		select {
		case <-ticker.C:
			request := stubs.TickerRequest{}
			response := new(stubs.TickerResponse)

			err := conn.Call(stubs.Ticker, request, response)

			checkerr(err, 45)

			mutex.Lock()
			d.events <- AliveCellsCount{response.Turns, response.Alive}
			mutex.Unlock()
		case c := <-done:
			flag = c
		}
	}
}

// Calls GolServer Increment inorder to get the world state
func GetWorld(p Params, world [][]uint8, conn *rpc.Client) *stubs.BoardResponse {
	params := stubs.StubsParams{p.Turns,p.Threads,p.ImageWidth,p.ImageHeight}

	request := stubs.BoardRequest{World: world, Params: params}
	response := new(stubs.BoardResponse)

	err := conn.Call(stubs.GameOfLifeHandler, request, response)

	checkerr(err,65)
	return response
}

//keypresses
func keypress(c distributorChannels, p Params, fileName string, mutex *sync.Mutex, conn *rpc.Client) {
	for {
		switch <-c.ioKeyPress {
		case 'p':
			request := stubs.KeyPRequest{false}
			response := new(stubs.KeyPResponse)
			err := conn.Call(stubs.P, request, response)
			checkerr(err,77)

			fmt.Println("Current turn ", response.Turn)

			mutex.Lock()
			c.events <- StateChange{CompletedTurns: response.Turn, NewState: Paused}
			mutex.Unlock()

			for <-c.ioKeyPress != 'p'{}

			request = stubs.KeyPRequest{true}
			err = conn.Call(stubs.P, request, response)
			checkerr(err,89)

			fmt.Println("Continuing")
			mutex.Lock()
			c.events <- StateChange{CompletedTurns: response.Turn, NewState: Executing}
			mutex.Unlock()

		case 'q':
			mutex.Lock()
			request := stubs.KeyRequest{}
			response := new(stubs.KeyQResponse)
			err := conn.Call(stubs.Q, request, response)
			checkerr(err,101)
		case 's':
			request := stubs.KeyRequest{}
			response := new(stubs.KeySResponse)
			err := conn.Call(stubs.S, request, response)
			checkerr(err,106)
			outputFile(fileName, c, p, response.World)
		case 'k':
			request := stubs.KeyRequest{}
			response := new(stubs.KeyKResponse)
			conn.Call(stubs.K, request, response)
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	// Sending name of file to io
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	world := inputFile(filename, c, p)

	turn := 0
	done := make(chan bool)

	conn, err := rpc.Dial("tcp", "localhost:8030")
	checkerr(err,126)
	defer conn.Close()

	var mutex = sync.Mutex{}

	go reportAlive(c,done, conn, &mutex)
	go keypress(c,p,filename,&mutex,conn)

	response := GetWorld(p, world, conn)

	turn = response.Turns
	cells := response.Alive
	world = response.World
	done <- true
	c.events <- FinalTurnComplete{turn, cells}

	outputFile(filename, c, p, world)

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// receives info from io.go inorder to make the world
func inputFile(filename string, c distributorChannels, p Params) [][]uint8 {
	c.ioCommand <- ioInput
	// Sending name of file to io
	c.ioFilename <- filename

	// stores the starting state of the world
	world := make([][]uint8, p.ImageHeight)
	for y := 0; y < p.ImageHeight; y++ {
		row := make([]uint8, p.ImageWidth)
		for x := 0; x < p.ImageWidth; x++ {
			row[x] = <-c.ioInput
			// send a cell fliped event
			if row[x] != 0 {
				c.events <- CellFlipped{CompletedTurns: 0, Cell: util.Cell{X: x, Y: y}}
			}
		}
		world[y] = row
	}
	return world
}

// sends info to io.go inorder to wright pmg file
func outputFile(filename string, c distributorChannels, p Params, world [][]uint8) {
	// Output File
	c.ioCommand <- ioOutput
	c.ioFilename <- filename + "x" + strconv.Itoa(p.Turns)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}
}
