package stubs

import "uk.ac.bris.cs/gameoflife/util"

var NodeStep = "GameOfLifeBoard.NextStep"
var GameOfLifeHandler = "Broker.Increment"
var Ticker = "Broker.Report"
var P = "Broker.KeyP"
var Q = "Broker.KeyQ"
var K = "Broker.KeyK"
var S = "Broker.KeyS"

var Shutdown = "GameOfLifeBoard.Shutdown"

var Subscribe = "Broker.Subscribe"

type GameBoard struct {
	World [][]uint8
	Turns int
}

type StubsParams struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

type SliceParams struct {
	ImageWidth 	int
	ImageHeight int
	Place		int
}

type Topic struct {
	Params StubsParams
	Result chan NewWorld
}


type TickerRequest struct {

}
type TickerResponse struct {
	Alive int
	Turns int
}


type BoardResponse struct {
	World [][]uint8
	Alive []util.Cell
	Turns int
	Quitting bool
}

type BoardRequest struct {
	World [][]uint8
	Params StubsParams
}

type IncrementRequest struct {
	World [][]uint8
	StartHeight  int
	EndHeight    int
	Width        int
	ActualHeight int
	TopWrap      bool
	BottomWrap   bool
}
type IncrementResponse struct {
	World [][]uint8
}

type KeyPRequest struct {
	Paused bool
}
type KeyPResponse struct {
	Turn int
}

type KeySResponse struct {
	World [][]uint8
}

type KeyKResponse struct {

}
type KeyQResponse struct {

}

type KeyRequest struct {

}

type PublishRequest struct {
	Topic string
	Params StubsParams

}


type ChannelRequest struct {
	Topic string
	Buffer int
}

type SubscriptionRequest struct {
	FactoryAddress string
}


type NewWorld struct {
	Result [][]uint8
	Turn   int
}

type StatusReport struct {
	Message string
}







