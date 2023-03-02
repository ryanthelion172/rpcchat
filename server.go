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

	// accept incoming connections and handle RPC requests
	for {

		conn, err := listener.Accept()
		if err != nil {
			log.Println("Failed to accept connection: ", err)
			continue
		}
		go handleConnection(conn)

	}
	// set up network listen and accept loop here
	// to receive and dispatch RPC requests
	// ...

	// wait for a shutdown request
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	msgType, err := ReadUint16(conn)
	if err != nil {
		log.Printf("error decoding message type: %v", err)
		return
	}
	switch msgType {
	case MsgRegister:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Register message: %v", err)
			return
		}
		if err := Register(user); err != nil {
			log.Printf("error handling Register message: %v", err)
		}
	case MsgList:
		users := List()
		err = WriteStringSlice(conn, users)
		if err != nil {
			log.Printf("error encoding List response: %v", err)
		}
		err = WriteString(conn, "")
		if err != nil {
			log.Printf("error encoding List response: %v", err)
		}
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
	case MsgQuit:
		user, err := ReadString(conn)
		if err != nil {
			log.Printf("error decoding Quit message: %v", err)
			return
		}
		Quit(user)
	case MsgShutdown:
		Shutdown()
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
	log.Print("tell <user> message: messages user directly\n" +
		"say: says message to all users\n" +
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

func waitAndCheck(server, user string, stop *programQuit) error {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return err
	}
	for stop.Quit == false {
		messages, err := CheckMessagesRPC(server, user)
		if err != nil {
			log.Fatal(err)
		}
		log.Print(messages)
		time.Sleep(1000)
	}
	return nil
}

func RegisterRPC(server, user string) error {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return err
	}
	Register(user)
	return nil
}

func ListRPC(server string) ([]string, error) {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return make([]string, 0), err
	}
	return List(), nil
}

func CheckMessagesRPC(server, user string) ([]string, error) {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return make([]string, 0), err
	}
	return CheckMessages(user), nil
}

func TellRPC(server, user, target, message string) error {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return err
	}
	Tell(user, target, message)
	return nil
}

func SayRPC(server, user, message string) error {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return err
	}
	Say(user, message)
	return nil
}

func QuitRPC(server, user string) error {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return err
	}
	Quit(user)
	return nil
}

func ShutdownRPC(server string) error {
	conn, err := net.Dial("tcp", server)
	defer conn.Close()
	if err != nil {
		return err
	}
	Shutdown()
	return nil
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
			log.Print(commands)
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
	buf := make([]byte, 2)
	buf[0] = byte(value >> 8)
	buf[1] = byte(value)
	_, err := conn.Write(buf)
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
func WriteString(w io.Writer, str string) error {
	if err := WriteUint16(w, uint16(len(str))); err != nil {
		return err
	}
	_, err := w.Write([]byte(str))
	return err
}

func WriteStringSlice(w io.Writer, strs []string) error {
	if err := WriteUint16(w, uint16(len(strs))); err != nil {
		return err
	}
	for _, str := range strs {
		if err := WriteString(w, str); err != nil {
			return err
		}
	}
	return nil
}
