package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"uk.ac.bris.cs/gameoflife/stubs"
)

func checkerr(e error, lineno int){
	if e != nil {
		fmt.Fprintf(os.Stderr, "Error occured: ", e, " ", lineno )
		os.Exit(2)
	}
}


func checkNeighbours(x int, y int, p stubs.IncrementRequest) uint8{
	noNeighbours := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i != 0 || j != 0 {
				ny := y + i
				nx := x + j

				//if p.TopWrap && ny < 0 {
				//	ny = p.ActualHeight - 1
				//}
				//if p.BottomWrap && ny >= p.StartHeight + p.StartHeight-p.EndHeight {
				//	ny = 0
				//}
				if nx < 0 {
					nx = p.Width - 1
				}

				if nx >= p.Width {
					nx = 0
				}
				if p.World[ny][nx] == 255 {
					noNeighbours++
				}

			}
		}
	}

	if p.World[y][x] == 0 {
		if noNeighbours == 3 {
			return 255
		}
	} else {
		if noNeighbours < 2 {
			return 0
		} else if noNeighbours <= 3 {
			return p.World[y][x]
		} else {
			return 0
		}
	}
	return 0
}


func calculateNextState(p stubs.IncrementRequest) [][]uint8 {
	newWorld := make([][]uint8, p.EndHeight-p.StartHeight)
	for y := 1; y < p.EndHeight-p.StartHeight+1; y++ {
		row := make([]uint8, p.Width)
		for x := 0; x < p.Width; x++ {
			k := checkNeighbours(x, y, p)
			row[x] = k
		}
		newWorld[y-1] = row
	}

	return newWorld
}


type GameOfLifeBoard struct {
	listener net.Listener
}



func (s *GameOfLifeBoard) NextStep(req stubs.IncrementRequest, res *stubs.IncrementResponse) (err error){
	res.World = calculateNextState(req)
	return
}

func (s *GameOfLifeBoard) Shutdown(req stubs.StatusReport, res *stubs.StatusReport) (err error){
	s.listener.Close()
	return
}


func main() {
	pAddr := flag.String("ip", "127.0.0.1:8050", "IP and port to listen on")
	//sAddr := flag.String("address", "localhost:8050", "IP and port of the server")
	brokerAddr := flag.String("broker","127.0.0.1:8030", "Address of broker instance")
	flag.Parse()

	g := GameOfLifeBoard{}

	rpc.Register(&g)

	listener, err := net.Listen("tcp", *pAddr)
	checkerr(err, 97)
	g.listener = listener
	defer listener.Close()

	client, err := rpc.Dial("tcp", *brokerAddr)
	checkerr(err, 101)

	request := stubs.SubscriptionRequest{
		FactoryAddress: *pAddr,
	}
	response := new(stubs.StatusReport)
	client.Call(stubs.Subscribe, request, response)


	rpc.Accept(listener)



}