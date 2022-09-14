package serv

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

//Cell is the same as in util.Cell
type Cell struct {
	X, Y int
}

const alive = 255
const dead = 0

/////////////////////////////////////

const sendWritePgmCode = "1\n"
const sendAliveCellsCode = "2\n"
const sendFinalTurnCompleteCode = "3\n"
const sendCloseProgramCode = "4\n"
const sendPauseProgramCode = "5\n"
const sendExecuteProgramCode = "6\n"
const sendTurnCompleteCode = "7\n"

////////////////////////////////////

func mod(x, m int) int {
	return (x + m) % m
}

// Return all alive cells. Send them to the client
func getCurrentAliveCells(world [][]uint8) []Cell {
	var cells []Cell

	for i, x := range world {
		for j, y := range x {
			if y != 0 {
				cells = append(cells, Cell{
					X: i,
					Y: j,
				})
			}
		}
	}

	return cells
}

func calculateNeighbours(x, y int, world [][]uint8) int {
	height := len(world)
	width := len(world[0])

	neighbours := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i != 0 || j != 0 {
				if world[mod(x+i, height)][mod(y+j, width)] == alive {
					neighbours++
				}
			}
		}
	}
	return neighbours
}

func calculateNextWorld(chunk chan [][]uint8, turn int, offset int) {
	world := <-chunk

	height := len(world)
	width := len(world[0])

	newWorld := make([][]byte, height)
	for i := range newWorld {
		newWorld[i] = make([]byte, width)
	}

	for x := 1; x < height-1; x++ {
		for y := 0; y < width; y++ {
			neighbours := calculateNeighbours(x, y, world)
			if world[x][y] == alive {
				if neighbours == 2 || neighbours == 3 {
					newWorld[x][y] = alive
				} else {
					//Send information to the controller
					// c.events <- CellFlipped{
					// 	CompletedTurns: turn,
					// 	Cell:           util.Cell{X: x + offset - 1, Y: y},
					// }
					newWorld[x][y] = dead
				}
			} else {
				if neighbours == 3 {
					//Send infrmation to controller
					// c.events <- CellFlipped{
					// 	CompletedTurns: turn,
					// 	Cell:           util.Cell{X: x + offset - 1, Y: y},
					// }
					newWorld[x][y] = alive
				} else {
					newWorld[x][y] = dead
				}
			}
		}
	}
	newWorld = newWorld[1:(height - 1)]
	chunk <- newWorld
}

func calculateDistributedStep(p Params, turn int, world [][]uint8) [][]uint8 {

	chunk := make([]chan [][]byte, p.Threads)
	worldsChunk := make([][][]uint8, p.Threads)

	chunkWidth := p.ImageWidth / p.Threads

	if p.Threads == 1 {
		worldsChunk[0] = append([][]byte{world[p.ImageWidth-1]}, world...)
		worldsChunk[0] = append(worldsChunk[0], [][]byte{world[0]}...)
	} else {
		//Making the world for the first thread
		worldsChunk[0] = append([][]byte{world[p.ImageWidth-1]}, world[0:chunkWidth+1]...)
		// Making the world for the last thread (if there is more than one thread)
		worldsChunk[p.Threads-1] = append(world[((p.Threads-1)*chunkWidth-1):], [][]byte{world[0]}...)
	}

	var newWorld [][]byte
	for i := 0; i < p.Threads; i++ {
		// ((chunkWidth * i) - 1) -> (chunkWidth * (i+1))

		if i != 0 && i != p.Threads-1 {
			worldsChunk[i] = world[(chunkWidth*i - 1):(chunkWidth*(i+1) + 1)]
		}
		offset := i * chunkWidth
		chunk[i] = make(chan [][]byte)
		go calculateNextWorld(chunk[i], turn, offset)
		chunk[i] <- worldsChunk[i]
	}

	for i := 0; i < p.Threads; i++ {
		newWorld = append(newWorld, <-chunk[i]...)
	}

	return newWorld
}

func sendCloseProgram(conn *net.Conn, turn *int) {
	closeString := sendCloseProgramCode + strconv.Itoa(*turn) + "\n"
	fmt.Fprintf(*conn, closeString)
}

func sendPauseProgram(conn *net.Conn, turn *int) {
	pauseString := sendPauseProgramCode + strconv.Itoa(*turn) + "\n"
	fmt.Fprintf(*conn, pauseString)
}

func sendExecutingProgram(conn *net.Conn, turn *int) {
	executeString := sendExecuteProgramCode + strconv.Itoa(*turn) + "\n"
	fmt.Fprintf(*conn, executeString)
}

//Receive key presses from controller
func manageSdlInput(p Params, conn *net.Conn, keyPresses <-chan rune, turn *int, world *[][]uint8, done chan bool, ticker *ticker) bool {
	select {
	case key := <-keyPresses:
		if key == 's' {
			sendWritePgm(conn, *turn, *world)
		} else if key == 'q' {
			sendWritePgm(conn, *turn, *world)
			sendCloseProgram(conn, turn)
			closeProgramm(*turn, done, ticker)
			return true
		} else if key == 'p' {
			ticker.stopTicker(done)
			sendPauseProgram(conn, turn)
			resume := 'N'
			for resume != 'p' {
				resume = <-keyPresses
				if resume == 's' {
					sendWritePgm(conn, *turn, *world)
				} else if resume == 'q' {
					sendWritePgm(conn, *turn, *world)
					sendCloseProgram(conn, turn)
					return true
				}
			}
			ticker.resetTicker(conn, turn, world, done)
			fmt.Println("Continuing")
			sendExecutingProgram(conn, turn)
		}
	default:
		return false
	}
	return false
}

func closeProgramm(turn int, done chan bool, ticker *ticker) {
	ticker.ticker.Stop()
	done <- true
}

type ticker struct {
	period time.Duration
	ticker time.Ticker
}

func createTicker(period time.Duration) *ticker {
	return &ticker{period, *time.NewTicker(period)}
}

func (t *ticker) stopTicker(done chan bool) {
	t.ticker.Stop()
	done <- true
}

func (t *ticker) resetTicker(conn *net.Conn, turn *int, world *[][]uint8, done chan bool) {
	t.ticker = *time.NewTicker(t.period)
	tickerRun(conn, turn, world, done, t)
}

func sendAliveCellsCount(conn *net.Conn, turn int, world [][]uint8) {
	finalString := sendAliveCellsCode + strconv.Itoa(turn) + "\n" + strconv.Itoa(len(getCurrentAliveCells(world))) + "\n"
	fmt.Fprintf(*conn, finalString)
}

func tickerRun(conn *net.Conn, turn *int, world *[][]uint8, done chan bool, ticker *ticker) {
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.ticker.C:
				sendAliveCellsCount(conn, *turn, *world)
			}
		}
	}()
}

func sendWritePgm(conn *net.Conn, turn int, world [][]byte) {
	writePgmString := sendWritePgmCode + strconv.Itoa(turn) + "\n"

	cells := getCurrentAliveCells(world)
	for _, cell := range cells {
		writePgmString = writePgmString + strconv.Itoa(cell.X) + " " + strconv.Itoa(cell.Y) + " "
	}

	writePgmString = writePgmString + "\n"

	fmt.Fprintf(*conn, writePgmString)
}

func sendFinalTurnComplete(conn *net.Conn, turn int, world [][]byte) {
	finalTurnCompleteString := sendFinalTurnCompleteCode + strconv.Itoa(turn) + "\n"

	cells := getCurrentAliveCells(world)
	for _, cell := range cells {
		finalTurnCompleteString = finalTurnCompleteString + strconv.Itoa(cell.X) + " " + strconv.Itoa(cell.Y) + " "
	}

	finalTurnCompleteString = finalTurnCompleteString + "\n"

	fmt.Fprintf(*conn, finalTurnCompleteString)
}

func sendTurnComplete(conn *net.Conn, turn int, world [][]byte) {
	sendTurnCompleteString := sendTurnCompleteCode + strconv.Itoa(turn) + "\n"

	cells := getCurrentAliveCells(world)
	for _, cell := range cells {
		sendTurnCompleteString = sendTurnCompleteString + strconv.Itoa(cell.X) + " " + strconv.Itoa(cell.Y) + " "
	}

	sendTurnCompleteString = sendTurnCompleteString + "\n"

	fmt.Fprintf(*conn, sendTurnCompleteString)
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, world [][]byte, conn *net.Conn, keyPresses <-chan rune) {

	turn := 0

	ticker := createTicker(2 * time.Second)
	done := make(chan bool)

	tickerRun(conn, &turn, &world, done, ticker)

	for turn < p.Turns {
		if manageSdlInput(p, conn, keyPresses, &turn, &world, done, ticker) {
			return
		}

		world = calculateDistributedStep(p, turn, world)

		turn++

		//sendTurnComplete(conn, turn, world)

		//Send information to the controller
		// c.events <- TurnComplete{
		// 	CompletedTurns: turn,
		// }
	}

	sendWritePgm(conn, turn, world)
	sendFinalTurnComplete(conn, turn, world)

	closeProgramm(turn, done, ticker)
}
