package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	MsgRegister = iota
	MsgList
	MsgCheckMessages
	MsgTell
	MsgSay
	MsgQuit
	MsgShutdown
)

type programQuit struct {
	Quit bool
}

var mutex sync.Mutex
var messages map[string][]string
var shutdown chan struct{}

func server(listenAddress string) {
	shutdown = make(chan struct{})
	messages = make(map[string][]string)

	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatal("Failed to listen on ", listenAddress, ": ", err)
	}
	defer listener.Close()

	// accept incoming connections and handle RPC requestsfor {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Failed to accept connection: ", err)
			continue
		}
		go handleConnection(conn)
	}

	// wait for a shutdown request
	<-shutdown
	time.Sleep(100 * time.Millisecond)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	msgType, err := ReadUint16(conn)
	if err != nil {
		log.Printf("error decoding message type: %v", err)
	}
	switch msgType {
	case MsgRegister:
		user, err := ReadString(conn)
		myErr := ""
		if err != nil {
			log.Printf("error decoding Register message: %v", err)
			myErr = "Uh oh"
			return
		}
		if err := Register(user); err != nil {
			log.Printf("error handling Register message: %v", err)
		}
		WriteString(conn, myErr)
	case MsgList:
		users := List()
		err = WriteStringSlice(conn, users)
		if err != nil {
			log.Printf("error encoding List response: %v", err)
		}

		WriteString(conn, "")

	case MsgCheckMessages:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding CheckMessages message: %v", err)
			return
		}
		messages := CheckMessages(user)
		err = WriteStringSlice(conn, messages)
		if err != nil {
			log.Printf("error encoding CheckMessages response: %v", err)
		}
		WriteString(conn, "")
	case MsgTell:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Tell message: %v", err)
			return
		}
		target, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Tell message: %v", err)
			return
		}
		message, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Tell message: %v", err)
			return
		}
		err = WriteString(conn, "")
		if err != nil {
			log.Printf("error encoding List response: %v", err)
		}
		Tell(user, target, message)
		WriteString(conn, "")
	case MsgSay:

		user, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Say message: %v", err)
			return
		}
		message, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Say message: %v", err)
			return
		}
		Say(user, message)
		WriteString(conn, "")
	case MsgQuit:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Quit message: %v", err)
			return
		}
		Quit(user)
		WriteString(conn, "")
	case MsgShutdown:
		Shutdown()
		WriteString(conn, "")
	default:
		log.Printf("unknown message type: %d", msgType)
	}

	<-shutdown
	time.Sleep(100 * time.Millisecond)
}

func Register(user string) error {
	if len(user) < 1 || len(user) > 20 {
		return fmt.Errorf("Register: user must be between 1 and 20 letters")
	}
	for _, r := range user {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("Register: user must only contain letters and digits")
		}
	}
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("*** %s has logged in", user)
	log.Printf(msg)
	for target, queue := range messages {
		messages[target] = append(queue, msg)
	}
	messages[user] = nil

	return nil
}

func List() []string {
	mutex.Lock()
	defer mutex.Unlock()

	var users []string
	for target := range messages {
		users = append(users, target)
	}
	sort.Strings(users)

	return users
}

func CheckMessages(user string) []string {
	mutex.Lock()
	defer mutex.Unlock()

	if queue, present := messages[user]; present {
		messages[user] = nil
		return queue
	} else {
		return []string{"*** You are not logged in, " + user}
	}
}

func Tell(user, target, message string) {
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("%s tells you %s", user, message)
	if queue, present := messages[target]; present {
		messages[target] = append(queue, msg)
	} else if queue, present := messages[user]; present {
		messages[user] = append(queue, "*** No such user: "+target)
	}
}

func Say(user, message string) {
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("%s says %s", user, message)
	for target, queue := range messages {
		messages[target] = append(queue, msg)
	}
}

func Help() {
	log.Print("tell <user> <message>: messages user directly\n" +
		"say <message>: says message to all users\n" +
		"list: Shows list of users\n" +
		"quit: Quits proram\n" +
		"shutdown: Shutdown server\n")

}

func Quit(user string) {
	mutex.Lock()
	defer mutex.Unlock()

	msg := fmt.Sprintf("*** %s has logged out", user)
	log.Print(msg)
	for target, queue := range messages {
		messages[target] = append(queue, msg)
	}
	delete(messages, user)
}

func Shutdown() {
	shutdown <- struct{}{}
}

func waitAndCheck(server, user string, stop *programQuit) {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	for stop.Quit == false {
		messages, err := CheckMessagesRPC(server, user)
		if err != nil {
			log.Fatal(err)
		}
		for _, message := range messages {
			log.Print(message)
		}
		time.Sleep(time.Second)
	}
}

func RegisterRPC(server, user string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Call register on server using WriteUint16.
	WriteUint16(conn, MsgRegister)
	// Give server parameter of user using WriteString.
	WriteString(conn, user)
	// receive error using ReadString.
	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func ListRPC(server string) ([]string, error) {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return make([]string, 0), err
	}
	defer conn.Close()
	// Call List on server using WriteUint16.
	WriteUint16(conn, MsgList)
	// receive returned list using ReadStringSlice.
	list, err := ReadStringSlice(conn)
	// receive error using ReadString.
	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return list, err

}

func CheckMessagesRPC(server, user string) ([]string, error) {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return make([]string, 0), err
	}
	defer conn.Close()
	// Call Check messages on server using WriteUint16.
	WriteUint16(conn, MsgCheckMessages)
	// Give server parameter of user using WriteString.
	WriteString(conn, user)
	// receive returned list using ReadStringSlice.
	message, err := ReadStringSlice(conn)
	// receive error using ReadString.

	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return message, err
}

func TellRPC(server, user, target, message string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Call Tell on server using WriteUint16.
	WriteUint16(conn, MsgTell)
	// Give server parameter of user, target, and message using WriteString.
	WriteString(conn, user)
	WriteString(conn, target)
	WriteString(conn, message)
	// receive error using ReadString.
	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func SayRPC(server, user, message string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Call Say on server using WriteUint16.
	WriteUint16(conn, MsgSay)
	// Give server parameter of user and message using WriteString.
	WriteString(conn, user)
	WriteString(conn, message)
	// receive error using ReadString.
	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func QuitRPC(server, user string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Call Quit on server using WriteUint16.
	WriteUint16(conn, MsgQuit)
	// Give server parameter of user using WriteString.
	WriteString(conn, user)
	// receive error using ReadString.
	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func ShutdownRPC(server string) error {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Call Shutdown on server using WriteUint16.
	WriteUint16(conn, MsgShutdown)
	// receive error using ReadString.
	errString, err := ReadString(conn)
	if errString != "" {
		log.Fatal(errString)
	}
	return err
}

func client(serverAddress, user string) {
	stop := programQuit{Quit: false}
	//connect to server
	err := RegisterRPC(serverAddress, user)
	if err != nil {
		log.Fatal(err)
	}
	go waitAndCheck(serverAddress, user, &stop)
	// read inputs
	for stop.Quit == false {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		err := scanner.Err()
		if err != nil {
			log.Fatal(err)
		}
		s := strings.Split(scanner.Text(), " ")
		switch s[0] {
		case "list":
			commands, err := ListRPC(serverAddress)
			if err != nil {
				log.Fatal("Command could not be sent, Quitting program.")
			}
			for _, command := range commands {
				log.Print(command)
			}
		case "tell":
			//recreate tell message
			originalMessage := ""
			for i, word := range s {
				if i >= 2 {
					originalMessage += word
					if i != len(s)-1 {
						originalMessage += " "
					}
				}
			}
			err := TellRPC(serverAddress, user, s[1], originalMessage)
			if err != nil {
				log.Fatal("Command could not be sent, Quitting program.")
			}
		case "say":
			//recreate say message
			originalMessage := ""
			for i, word := range s {
				if i >= 1 {
					originalMessage += word
					if i != len(s)-1 {
						originalMessage += " "
					}
				}
			}
			err := SayRPC(serverAddress, user, originalMessage)
			if err != nil {
				log.Fatal("Command could not be sent, Quitting program.")
			}
		case "quit":
			stop.Quit = true
			err := QuitRPC(serverAddress, user)
			if err != nil {
				log.Fatal("Command could not be sent, Quitting program.")
			}
		case "shutdown":
			stop.Quit = true
			err := ShutdownRPC(serverAddress)
			if err != nil {
				log.Fatal("Command could not be sent, Quitting program.")
			}
		case "help":
			Help()
		case "":
		default:
			log.Print("Error: Unrecognized command")
			Help()
		}
	}
}

func main() {
	log.SetFlags(log.Ltime)

	var listenAddress string
	var serverAddress string
	var username string

	switch len(os.Args) {
	case 2:
		listenAddress = net.JoinHostPort("", os.Args[1])
	case 3:
		serverAddress = os.Args[1]
		if strings.HasPrefix(serverAddress, ":") {
			serverAddress = "localhost" + serverAddress
		}
		username = strings.TrimSpace(os.Args[2])
		if username == "" {
			log.Fatal("empty user name")
		}
	default:
		log.Fatalf("Usage: %s <port>   OR   %s <server> <user>",
			os.Args[0], os.Args[0])
	}

	if len(listenAddress) > 0 {
		server(listenAddress)
	} else {
		client(serverAddress, username)
	}
}

func ReadUint16(r io.Reader) (uint16, error) {
	buf := make([]byte, 2)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}

	value := uint16(buf[0])<<8 | uint16(buf[1])
	return value, nil
}
func WriteUint16(conn io.Writer, value uint16) error {
	raw := []byte{
		byte((value >> 8) & 0xff),
		byte((value >> 0) & 0xff),
	}
	_, err := conn.Write(raw)
	return err
}
func WriteString(conn io.Writer, value string) error {
	WriteUint16(conn, uint16(len(value)))
	_, err := io.WriteString(conn, value)
	return err
}

func WriteStringSlice(conn io.Writer, value []string) error {
	err := WriteUint16(conn, uint16(len(value)))
	for _, x := range value {
		err = WriteString(conn, x)
	}
	return err
}
func ReadString(r io.Reader) (string, error) {
	strLen, err := ReadUint16(r)
	if err != nil {
		return "", err
	}
	strBuf := make([]byte, strLen)
	_, err = io.ReadFull(r, strBuf)
	if err != nil {
		return "", err
	}
	return string(strBuf), nil
}
func ReadStringSlice(r io.Reader) ([]string, error) {
	strsLen, err := ReadUint16(r)
	if err != nil {
		return nil, err
	}
	strs := make([]string, strsLen)
	for i := uint16(0); i < strsLen; i++ {
		str, err := ReadString(r)
		if err != nil {
			return nil, err
		}
		strs[i] = str
	}
	return strs, nil
}
