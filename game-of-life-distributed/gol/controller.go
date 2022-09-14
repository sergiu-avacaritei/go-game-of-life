package gol

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"

	"uk.ac.bris.cs/gameoflife/util"
)

//////////////////

const ImageOutputCompleteEvent = 1
const AliveCellsCountEvent = 2
const FinalTurnCompleteEvent = 3
const QuittingEvent = 4
const PauseEvent = 5
const ExecutingEvent = 6
const TurnCompleteEvent = 7

/////////////////

type distributorChannels struct {
	events        chan<- Event
	ioCommand     chan<- ioCommand
	ioIdle        <-chan bool
	ioFilename    chan<- string
	ioOutput      chan<- uint8
	ioInput       <-chan uint8
	sdlKeyPresses <-chan rune
}

func writePgm(p Params, c distributorChannels, turn int, world [][]byte) {
	fileName := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)
	c.ioCommand <- 0
	c.ioFilename <- fileName
	for x := 0; x < p.ImageHeight; x++ {
		for y := 0; y < p.ImageWidth; y++ {
			c.ioOutput <- world[y][x]
		}
	}
	c.events <- ImageOutputComplete{turn, fileName}
}

func getInitialWorld(p Params, c distributorChannels) [][]byte {

	initialWorld := make([][]byte, p.ImageHeight)
	for i := range initialWorld {
		initialWorld[i] = make([]byte, p.ImageWidth)
	}

	var aliveCells []util.Cell

	for x := 0; x < p.ImageHeight; x++ {
		for y := 0; y < p.ImageWidth; y++ {
			initialWorld[y][x] = <-c.ioInput // (Y,X) !!!!!!
			if initialWorld[y][x] == 255 {
				aliveCells = append(aliveCells, util.Cell{
					X: x,
					Y: y,
				})
				c.events <- CellFlipped{
					CompletedTurns: 0,
					Cell: util.Cell{
						X: y,
						Y: x,
					},
				}
			}
		}
	}

	return initialWorld
}

func closeProgramm(c distributorChannels, turn int, done chan<- bool) {

	// Make sure that the Io has finished any output before exiting.

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	done <- true
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func sendSdlInput(conn *net.Conn, c distributorChannels, done <-chan bool) {

	for {
		select {
		case key := <-c.sdlKeyPresses:
			sendKey := string(key) + "\n"
			fmt.Fprintf(*conn, sendKey)
		case <-done:
			return
		default:
		}
	}
}

func getCurrentAliveCells(world [][]uint8) []util.Cell {
	var cells []util.Cell

	for i, x := range world {
		for j, y := range x {
			if y != 0 {
				cells = append(cells, util.Cell{
					X: i,
					Y: j,
				})
			}
		}
	}

	return cells
}

func send(conn *net.Conn, p Params, world [][]byte) {
	// lets create the message we want to send accross
	var stringParams string

	stringParams = strconv.Itoa(p.ImageHeight) + " " + strconv.Itoa(p.ImageWidth) + " " + strconv.Itoa(p.Threads) + " " + strconv.Itoa(p.Turns) + "\n"

	// var stringWorld string

	// for x := range sendWorld {
	// 	if sendWorld[x] == 0 {
	// 		stringWorld = stringWorld + "0"
	// 	} else {
	// 		stringWorld = stringWorld + "1"
	// 	}
	// }

	var aliveCellsString string

	cells := getCurrentAliveCells(world)
	for _, cell := range cells {
		aliveCellsString = aliveCellsString + strconv.Itoa(cell.X) + " " + strconv.Itoa(cell.Y) + " "
	}

	aliveCellsString = aliveCellsString + "\n"

	sendMessage := stringParams + aliveCellsString

	fmt.Fprintf(*conn, sendMessage)
}

func createWorldAliveCells(p Params, aliveCells []util.Cell) [][]byte {
	initialWorld := make([][]byte, p.ImageHeight)
	for i := range initialWorld {
		initialWorld[i] = make([]byte, p.ImageWidth)
	}

	for _, x := range aliveCells {
		initialWorld[x.X][x.Y] = 255
	}

	return initialWorld
}

func makeFinalTurnComplete(conn *net.Conn, p Params, c distributorChannels, reader *bufio.Reader, done chan<- bool) {
	//fmt.Println("Sunt si in functie")
	turnsString, _ := reader.ReadString('\n')

	//fmt.Println("Numarul de ture: ", turnsString)
	turnsString = turnsString[:(len(turnsString))-1]
	turn, _ := strconv.Atoi(turnsString)

	aliveCellsString, _ := reader.ReadString('\n')
	//fmt.Println("Celule vii: ", aliveCellsString)

	aliveCellsArray := strings.Fields(aliveCellsString)

	var aliveCells []util.Cell

	for i := 0; i < len(aliveCellsArray); i = i + 2 {
		cell := util.Cell{}
		cell.X, _ = strconv.Atoi(aliveCellsArray[i])
		cell.Y, _ = strconv.Atoi(aliveCellsArray[i+1])
		aliveCells = append(aliveCells, cell)
	}

	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          aliveCells,
	}

	closeProgramm(c, turn, done)
	(*conn).Close()
}

func printAliveCells(conn *net.Conn, p Params, c distributorChannels, reader *bufio.Reader) {
	turnsString, _ := reader.ReadString('\n')
	//fmt.Println("Numarul de ture: ", turnsString)
	turnsString = turnsString[:(len(turnsString))-1]
	turn, _ := strconv.Atoi(turnsString)

	aliveCellsString, _ := reader.ReadString('\n')

	aliveCellsString = aliveCellsString[:(len(aliveCellsString))-1]
	aliveCells, _ := strconv.Atoi(aliveCellsString)

	c.events <- AliveCellsCount{turn, aliveCells}
}

func makeEventWritePgm(conn *net.Conn, p Params, c distributorChannels, reader *bufio.Reader) {
	//fmt.Println("Sunt si in functie")
	turnsString, _ := reader.ReadString('\n')

	//fmt.Println("Numarul de ture: ", turnsString)
	turnsString = turnsString[:(len(turnsString))-1]
	turn, _ := strconv.Atoi(turnsString)

	aliveCellsString, _ := reader.ReadString('\n')
	//fmt.Println("Celule vii: ", aliveCellsString)

	aliveCellsArray := strings.Fields(aliveCellsString)

	var aliveCells []util.Cell

	for i := 0; i < len(aliveCellsArray); i = i + 2 {
		cell := util.Cell{}
		cell.X, _ = strconv.Atoi(aliveCellsArray[i])
		cell.Y, _ = strconv.Atoi(aliveCellsArray[i+1])
		aliveCells = append(aliveCells, cell)
	}

	world := createWorldAliveCells(p, aliveCells)

	writePgm(p, c, turn, world)

}

func makeCloseProgramEvent(conn *net.Conn, p Params, c distributorChannels, reader *bufio.Reader, done chan<- bool) {
	turnsString, _ := reader.ReadString('\n')

	turnsString = turnsString[:(len(turnsString))-1]
	turn, _ := strconv.Atoi(turnsString)

	closeProgramm(c, turn, done)

}

func makeEventPauseProgram(conn *net.Conn, p Params, c distributorChannels, reader *bufio.Reader) {
	turnsString, _ := reader.ReadString('\n')

	turnsString = turnsString[:(len(turnsString))-1]
	turn, _ := strconv.Atoi(turnsString)

	c.events <- StateChange{turn, Paused}

}

func makeEventExecutingProgram(conn *net.Conn, p Params, c distributorChannels, reader *bufio.Reader) {
	turnsString, _ := reader.ReadString('\n')

	turnsString = turnsString[:(len(turnsString))-1]
	turn, _ := strconv.Atoi(turnsString)

	c.events <- StateChange{turn, Executing}

}

func makeTurnCompleteEvent(conn *net.Conn, c distributorChannels, reader *bufio.Reader, lastCells []util.Cell) []util.Cell {
	turnsString, _ := reader.ReadString('\n')

	turnsString = turnsString[:(len(turnsString))-1]
	turn, _ := strconv.Atoi(turnsString)

	aliveCellsString, _ := reader.ReadString('\n')
	//fmt.Println("Celule vii: ", aliveCellsString)

	aliveCellsArray := strings.Fields(aliveCellsString)

	var aliveCells []util.Cell

	for i := 0; i < len(aliveCellsArray); i = i + 2 {
		cell := util.Cell{}
		cell.X, _ = strconv.Atoi(aliveCellsArray[i])
		cell.Y, _ = strconv.Atoi(aliveCellsArray[i+1])
		aliveCells = append(aliveCells, cell)
	}

	for _, cell := range lastCells {
		c.events <- CellFlipped{
			turn,
			cell,
		}
	}

	for _, cell := range aliveCells {
		c.events <- CellFlipped{
			turn,
			cell,
		}
	}

	c.events <- TurnComplete{
		turn,
	}

	return aliveCells

}

// REFACTOR (Use w)
func receive(conn *net.Conn, c distributorChannels, p Params, done chan<- bool, lastCells []util.Cell) {
	reader := bufio.NewReader(*conn)
	for {

		codeChar, err := reader.ReadString('\n')
		if err != nil {
			(*conn).Close()
			return
		}
		if len(codeChar) > 0 {
			codeChar = codeChar[:(len(codeChar))-1]
			code, _ := strconv.Atoi(codeChar)

			switch code {
			case ImageOutputCompleteEvent:
				makeEventWritePgm(conn, p, c, reader)
			case AliveCellsCountEvent:
				printAliveCells(conn, p, c, reader)
			case FinalTurnCompleteEvent:
				makeFinalTurnComplete(conn, p, c, reader, done)
			case QuittingEvent:
				makeCloseProgramEvent(conn, p, c, reader, done)
			case PauseEvent:
				makeEventPauseProgram(conn, p, c, reader)
			case ExecutingEvent:
				makeEventExecutingProgram(conn, p, c, reader)
			case TurnCompleteEvent:
				lastCells = makeTurnCompleteEvent(conn, c, reader, lastCells)
			}

			// if code == 4 {
			// 	makeCloseProgramEvent(conn, p, c, reader)
			// } else if code == 3 {
			// 	makeFinalTurnComplete(conn, p, c, reader)
			// } else if code == 2 {
			// 	printAliveCells(conn, p, c, reader)
			// } else if code == 1 {
			// 	makeEventWritePgm(conn, p, c, reader)
			// } else if code == 5 {
			// 	makeEventPauseProgram(conn, p, c, reader)
			// } else if code == 6 {
			// 	makeEventExecutingProgram(conn, p, c, reader)
			// }
		}
	}
}

func controller(p Params, c distributorChannels) {

	fmt.Println("Intasi in controller")
	// READ
	c.ioCommand <- 1
	c.ioFilename <- fmt.Sprintf("%vx%v", p.ImageWidth, p.ImageHeight)

	// TODO: Create a 2D slice to store the world.
	// TODO: For all initially alive cells send a CellFlipped Event.
	world := getInitialWorld(p, c)
	//fmt.Println(p, world)

	conn, _ := net.Dial("tcp", "127.0.0.1:8030")

	done := make(chan bool)

	initialCells := getCurrentAliveCells(world)

	send(&conn, p, world)
	go receive(&conn, c, p, done, initialCells)
	go sendSdlInput(&conn, c, done)

	//send world to server

}
