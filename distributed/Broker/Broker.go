package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

func checkerr(e error, lineno int){
	if e != nil {
		fmt.Fprintf(os.Stderr, "Error occured: ", e, " At ", lineno)
		os.Exit(2)
	}
}

var (
	workers []*rpc.Client
	topicmx *sync.Mutex
)

// Calculate without modulo
func sendCalls(p stubs.StubsParams, world [][]uint8) [][]uint8 {

	var newWorld [][]uint8
	var returnWorld []*rpc.Call
	var responses []*stubs.IncrementResponse

	topicmx.Lock()
	workerHeight := p.ImageHeight / len(workers)
	for i, work := range workers{
		h := i*workerHeight + workerHeight
		w := p.ImageWidth
		var request stubs.IncrementRequest
		if i == 0 && i == len(workers)-1 {
			reducedWorld := world[len(world)-1:]
			reducedWorld = append(reducedWorld,world...)
			reducedWorld = append(reducedWorld, world[:1]...)
			request = stubs.IncrementRequest{reducedWorld, i * workerHeight, h, w, p.ImageHeight, true, true}
		} else if i == 0 {
			reducedWorld := world[len(world)-1:]
			reducedWorld = append(reducedWorld, world[i*workerHeight:h+1]...)
			request = stubs.IncrementRequest{reducedWorld, i * workerHeight, h, w, p.ImageHeight, true, false}
		} else if i == len(workers)-1 {
			reducedWorld := world[i*workerHeight-1:h + p.ImageHeight%len(workers)]
			reducedWorld = append(reducedWorld, world[:1]...)
			request = stubs.IncrementRequest{reducedWorld, i * workerHeight, h + p.ImageHeight%len(workers), w, p.ImageHeight, false, true}
		} else {
			reducedWorld := world[i*workerHeight-1:h+1]
			request = stubs.IncrementRequest{reducedWorld, i * workerHeight, h, w, p.ImageHeight, false, false}
		}
		response := new(stubs.IncrementResponse)

		worldCall := work.Go(stubs.NodeStep, request, response, nil)

		returnWorld = append(returnWorld, worldCall)
		responses = append(responses, response)

	}
	topicmx.Unlock()

	for i, r := range returnWorld{
		call := <-r.Done
		var response [][]uint8
		if call.Error != nil{
			topicmx.Lock()
			fmt.Println("Client disconnected!")
			workers = append(workers[:i], workers[i+1:]...)
			topicmx.Unlock()
			return nil
		} else {
			response = (*responses[i]).World
			newWorld = append(newWorld, response...)
		}
	}
	return newWorld
}

// Gives an array of all alive cell locations
func getAlive(p stubs.StubsParams, world [][]uint8) []util.Cell {
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

type Broker struct {
	b stubs.GameBoard
	p stubs.StubsParams
	mutex *sync.Mutex
	isConnected bool
	isAbleToQuit chan bool
	listener net.Listener

}

func (s *Broker) Increment(req stubs.BoardRequest, res *stubs.BoardResponse) (err error){
	if len(workers )<= 0{
		return errors.New("No servers have subscribed to the broker")
	}
	s.isConnected = true
	world := req.World
	turns := 0
	s.b = stubs.GameBoard{
		World: world,
		Turns: 0,
	}
	s.p =  req.Params


	for ; turns < req.Params.Turns; turns ++{
		if s.isConnected{
			s.mutex.Lock()
			s.b = stubs.GameBoard{
				World: world,
				Turns: turns,
			}
			s.mutex.Unlock()
			callWorld := sendCalls(req.Params, world)
			for ; callWorld == nil ; {
				callWorld = sendCalls(req.Params, world)
			}
			world = callWorld
		} else {
			res.World = world
			res.Turns = turns
			res.Alive = getAlive(req.Params, world)
			s.mutex.Lock()
			s.isAbleToQuit <- true
			s.mutex.Unlock()
			return
		}
	}
	res.Alive = getAlive(req.Params, world)
	res.World = world
	res.Turns = turns

	return
}

func (s *Broker) Report(_, res *stubs.TickerResponse) (err error){
	s.mutex.Lock()
	res.Alive = len(getAlive(s.p, s.b.World))
	res.Turns = s.b.Turns
	s.mutex.Unlock()

	return
}

func (s *Broker) KeyP(req *stubs.KeyPRequest, res *stubs.KeyPResponse) (err error){
	if !req.Paused {
		// is not paused
		s.mutex.Lock()
		res.Turn = s.b.Turns
	} else {
		// is paused
		res.Turn = s.b.Turns
		s.mutex.Unlock()
	}
	return
}
func (s *Broker) KeyQ(_, res *stubs.KeyQResponse) (err error){
	s.mutex.Lock()
	s.isConnected = false
	s.mutex.Unlock()
	<-s.isAbleToQuit
	return
}
func (s *Broker) KeyK(_, res *stubs.KeyKResponse) (err error){
	s.mutex.Lock()
	s.isConnected = false
	s.mutex.Unlock()

	<-s.isAbleToQuit
	// this doesn't exist (don't look)
	time.Sleep(1*time.Second)

	for _, w := range workers {
		request := new(stubs.StatusReport)
		response := new(stubs.StatusReport)

		w.Call(stubs.Shutdown, request, response)

	}

	s.mutex.Lock()
	s.listener.Close()
	s.mutex.Unlock()
	return
}
func (s *Broker) KeyS(_, res *stubs.KeySResponse) (err error){
	s.mutex.Lock()
	res.World = s.b.World
	s.mutex.Unlock()
	return
}

// Subscribe to worker nodes
func (s *Broker) Subscribe (req *stubs.SubscriptionRequest, res *stubs.StatusReport) (err error){
	fmt.Println(req)
	client, err := rpc.Dial("tcp", req.FactoryAddress)

	if err == nil {
		topicmx.Lock()
		workers = append(workers, client)
		topicmx.Unlock()
		res.Message = "Connected to " + req.FactoryAddress
	} else {
		fmt.Println("Error subscribing ", req.FactoryAddress)
		fmt.Println(err)
		return err
	}
	return
}




func main(){
	pAddr := flag.String("port","8030","Port to listen on")
	flag.Parse()
	topicmx = &sync.Mutex{}

	b := Broker{
		mutex: &sync.Mutex{},
		isAbleToQuit: make(chan bool),
	}

	rpc.Register(&b)

	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	b.listener = listener


	rpc.Accept(listener)




}
