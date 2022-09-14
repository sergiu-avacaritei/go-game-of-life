package serv

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

var server net.Listener

//Params is tmp
type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

//DataToSend is data to send
type initialInput struct {
	p     Params
	world [][]byte
}

func logerr(err error) bool {
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Println("read timeout:", err)
		} else if err == io.EOF {
		} else {
			log.Println("read error:", err)
		}
		return true
	}
	return false
}

func createWorldAliveCells(p Params, aliveCells []Cell) [][]byte {
	initialWorld := make([][]byte, p.ImageHeight)
	for i := range initialWorld {
		initialWorld[i] = make([]byte, p.ImageWidth)
	}

	for _, x := range aliveCells {
		initialWorld[x.X][x.Y] = 255
	}

	return initialWorld
}

func read(conn *net.Conn) (Params, [][]byte) {
	reader := bufio.NewReader(*conn)
	//fmt.Println("Am intrat in reader!!!")

	for {
		paramsString, _ := reader.ReadString('\n')
		if len(paramsString) > 0 {
			//fmt.Println("Astia sunt parametrii: ", paramsString)
			aliveCellsString, _ := reader.ReadString('\n')

			p := strings.Fields(paramsString)

			params := Params{}
			params.ImageHeight, _ = strconv.Atoi(p[0])
			params.ImageWidth, _ = strconv.Atoi(p[1])
			params.Threads, _ = strconv.Atoi(p[2])
			params.Turns, _ = strconv.Atoi(p[3])

			//fmt.Println("Celule vii: ", aliveCellsString)

			aliveCellsArray := strings.Fields(aliveCellsString)

			var aliveCells []Cell

			for i := 0; i < len(aliveCellsArray); i = i + 2 {
				cell := Cell{}
				cell.X, _ = strconv.Atoi(aliveCellsArray[i])
				cell.Y, _ = strconv.Atoi(aliveCellsArray[i+1])
				aliveCells = append(aliveCells, cell)
			}

			world := createWorldAliveCells(params, aliveCells)

			return params, world
		}
	}
}

func receiverSDL(conn *net.Conn, keyPresses chan<- rune) {
	reader := bufio.NewReader(*conn)
	for {
		codeChar, err := reader.ReadString('\n')
		if err != nil {
			(*conn).Close()
			return
		}
		if len(codeChar) > 0 {
			keyPresses <- rune(codeChar[0])
		}
	}
}

func handle(conn *net.Conn) {
	//timeoutDuration := 5 * time.Second
	fmt.Println("Launching server...")
	//(*conn).SetReadDeadline(time.Now().Add(timeoutDuration))

	remoteAddr := (*conn).RemoteAddr().String()
	fmt.Println("Client connected from " + remoteAddr)

	p, w := read(conn)

	keyPresses := make(chan rune, 10)

	go receiverSDL(conn, keyPresses)

	distributor(p, w, conn, keyPresses)

	(*conn).Close()
	server.Close()
	//resp(conn, d)
}

//RunServ runs the server
func RunServ() {
	server, _ = net.Listen("tcp", ":8030")
	//fmt.Println("I'm listening!")
	go func() {
		for {
			conn, err := server.Accept()
			//fmt.Println("kajdkabfsbakjshfdkasilasdkjhakfbasldkjasfkashfkbaskhajk")
			if err != nil {
				log.Println("Connection error: ", err)
				return
			}
			go handle(&conn)
		}
	}()
}
