package gol

import (
	"fmt"
	"strconv"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	ioKeyPress <-chan rune
}

type dimentions struct {
	startHeight  int
	endHeight    int
	width        int
	actualHeight int
	topWrap      bool
	bottomWrap   bool
}

type gameBoard struct {
	world [][]uint8
	turns int
}

type keyChannels struct {
	pause   chan bool
	world   chan gameBoard
	mutex   *sync.Mutex
	pauseNo int
}

func checkNeighbours(x int, y int, world [][]uint8, dim dimentions) uint8 {
	noNeighbours := 0

	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i != 0 || j != 0 {
				ny := y + i
				nx := x + j

				/*	if dim.topWrap && ny < 0 {
						ny = dim.actualHeight - 1
					}
					if dim.bottomWrap && ny >= dim.endHeight {
						ny = 0
					}*/
				if nx < 0 {
					nx = dim.width - 1
				}

				if nx >= dim.width {
					nx = 0
				}
				if world[ny][nx] == 255 {
					noNeighbours++
				}

			}
		}
	}

	if world[y][x] == 0 {
		if noNeighbours == 3 {
			return 255
		}
	} else {
		if noNeighbours < 2 {
			return 0
		} else if noNeighbours <= 3 {
			return world[y][x]
		} else {
			return 0
		}
	}
	return 0
}

func calculateSlice(dim dimentions, worldChan chan [][]uint8, channel chan [][]uint8, e chan<- Event) {
	world := <-worldChan
	turn := 0

	for world != nil {
		newWorld := make([][]uint8, dim.endHeight-dim.startHeight)
		for y := 1; y < dim.endHeight-dim.startHeight+1; y++ {
			row := make([]uint8, dim.width)
			for x := 0; x < dim.width; x++ {
				k := checkNeighbours(x, y, world, dim)
				if world[y][x] != k {
					e <- CellFlipped{turn, util.Cell{X: x, Y: y + dim.startHeight-1}}
				}
				row[x] = k
			}
			newWorld[y-1] = row

		}
		channel <- newWorld
		world = <-worldChan
		turn++
	}
}

// TODO: remember to split the world into different slices at some point
func calculateNextState(p Params, world [][]uint8, d distributorChannels, tickerChan chan gameBoard, mutex *sync.Mutex, kc keyChannels) gameBoard {

	workerHeight := p.ImageHeight / p.Threads

	// slice of channels to gol threads
	out := make([]chan [][]uint8, p.Threads)
	sendWorld := make([]chan [][]uint8, p.Threads)

	for i := 0; i < p.Threads; i++ {
		// might be worth benchmarking with different buffer size
		out[i] = make(chan [][]uint8)
		sendWorld[i] = make(chan [][]uint8)

		h := i*workerHeight + workerHeight
		w := p.ImageWidth
		var dim dimentions
		if i == 0 && i == p.Threads-1 {
			dim = dimentions{i * workerHeight, h, w, p.ImageHeight, true, true}
		} else if i == 0 {
			dim = dimentions{i * workerHeight, h, w, p.ImageHeight, true, false}
		} else if i == p.Threads-1 {
			dim = dimentions{i * workerHeight, h + p.ImageHeight%p.Threads, w, p.ImageHeight, false, true}
		} else {
			dim = dimentions{i * workerHeight, h, w, p.ImageHeight, false, false}
		}
		go calculateSlice(dim, sendWorld[i], out[i], d.events)
	}

	turn := 1

	for ; turn <= p.Turns; turn++ {
		var newWorld [][]uint8
		var reducedWorld [][]uint8
		for i := 0; i < p.Threads; i++ {
			h := i*workerHeight + workerHeight

			if i == 0 && i == p.Threads-1 {
				reducedWorld = world[len(world)-1:]
				reducedWorld = append(reducedWorld, world...)
				reducedWorld = append(reducedWorld, world[:1]...)
			} else if i == 0 {
				reducedWorld = world[len(world)-1:]
				reducedWorld = append(reducedWorld, world[i*workerHeight:h+1]...)
			} else if i == p.Threads-1 {
				reducedWorld = world[i*workerHeight-1 : h+p.ImageHeight%p.Threads]
				reducedWorld = append(reducedWorld, world[:1]...)
			} else {
				reducedWorld = world[i*workerHeight-1 : h+1]
			}
			sendWorld[i] <- reducedWorld
		}
		for i:=0; i < p.Threads; i++ {
			newWorld = append(newWorld, <-out[i]...)
		}
		world = newWorld
		if len(kc.world) > 0 {
			<-kc.world
		}
		mutex.Lock()
		kc.world <- gameBoard{world: world, turns: turn}
		tickerChan <- gameBoard{world: world, turns: turn}
		d.events <- TurnComplete{turn}
		mutex.Unlock()

	}
	return gameBoard{world, turn}
}

// Gives an array of all alive cell locations
func getAlive(p Params, world [][]uint8) []util.Cell {
	var cells []util.Cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				cells = append(cells, util.Cell{X: x, Y: y})
			}
		}
	}
	return cells
}

// Sends an alive cells count event every 2 seconds
func reportAlive(p Params, gb chan gameBoard, d distributorChannels, mutex *sync.Mutex, done chan bool) {
	ticker := time.NewTicker(2 * time.Second)
	flag := false
	for !flag {
		select {
		case <-ticker.C:
			mutex.Lock()
			world := <-gb
			mutex.Unlock()

			alive := getAlive(p, world.world)

			mutex.Lock()
			d.events <- AliveCellsCount{world.turns, len(alive)}
			mutex.Unlock()
		case c := <-done:
			flag = c
		default:
			for len(gb) > 1 {
				<-gb
			}
		}
	}
}

// keypresses
func keypress(c distributorChannels, p Params, fileName string, kc keyChannels) {
	for {
		switch <-c.ioKeyPress {
		case 'p':
			kc.mutex.Lock()
			world := <-kc.world
			c.events <- StateChange{CompletedTurns: world.turns, NewState: Paused}

			for <-c.ioKeyPress != 'p' {
			}

			fmt.Println("Continuing")
			c.events <- StateChange{CompletedTurns: world.turns, NewState: Executing}
			kc.mutex.Unlock()

		case 'q':

			world := <-kc.world
			kc.mutex.Lock()
			outputFile(fileName, c, p, world.world)
			// need to send a message to stop the calculating of turns!!!
			c.ioCommand <- ioCheckIdle
			<-c.ioIdle

			c.events <- StateChange{world.turns, Quitting}
			close(c.events)
		case 's':
			outputFile(fileName, c, p, (<-kc.world).world)
		case 'k':
			// not used for parallel
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	world := inputFile(filename, c, p)
	turn := 0

	var mutex = sync.Mutex{}

	kc := keyChannels{
		pause:   make(chan bool, 2),
		world:   make(chan gameBoard, p.Threads+1),
		mutex:   &mutex,
		pauseNo: 0,
	}

	go keypress(c, p, filename, kc)

	kc.world <- gameBoard{world: world, turns: 0}

	tickerChan := make(chan gameBoard, p.Threads+1)

	done := make(chan bool, 3)
	go reportAlive(p, tickerChan, c, &mutex, done)
	tickerChan <- gameBoard{world, 0}

	if p.Turns > 0 {
		value := calculateNextState(p, world, c, tickerChan, &mutex, kc)
		turn = value.turns
		world = value.world
	}

	done <- true

	cells := getAlive(p, world)
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
