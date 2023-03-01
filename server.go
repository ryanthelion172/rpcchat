package main

import (
	"bufio"
	"encoding/gob"
	"fmt"
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
		select {
		case <-shutdown:
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				log.Println("Failed to accept connection: ", err)
				continue
			}
			go handleConnection(conn)
		}
	}
	// set up network listen and accept loop here
	// to receive and dispatch RPC requests
	// ...

	// wait for a shutdown request
	time.Sleep(100 * time.Millisecond)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	dec := gob.NewDecoder(conn)
	var msgType int
	if err := dec.Decode(&msgType); err != nil {
		log.Printf("error decoding message type: %v", err)
		return
	}
	switch msgType {
	case MsgRegister:
		var user string
		if err := dec.Decode(&user); err != nil {
			log.Printf("error decoding Register message: %v", err)
			return
		}
		if err := Register(user); err != nil {
			log.Printf("error handling Register message: %v", err)
		}
	case MsgList:
		users := List()
		enc := gob.NewEncoder(conn)
		if err := enc.Encode(users); err != nil {
			log.Printf("error encoding List response: %v", err)
		}
	case MsgCheckMessages:
		var user string
		if err := dec.Decode(&user); err != nil {
			log.Printf("error decoding CheckMessages message: %v", err)
			return
		}
		messages := CheckMessages(user)
		enc := gob.NewEncoder(conn)
		if err := enc.Encode(messages); err != nil {
			log.Printf("error encoding CheckMessages response: %v", err)
		}
	case MsgTell:
		var user, target, message string
		if err := dec.Decode(&user); err != nil {
			log.Printf("error decoding Tell message: %v", err)
			return
		}
		if err := dec.Decode(&target); err != nil {
			log.Printf("error decoding Tell message: %v", err)
			return
		}
		if err := dec.Decode(&message); err != nil {
			log.Printf("error decoding Tell message: %v", err)
			return
		}
		Tell(user, target, message)
	case MsgSay:
		var user, message string
		if err := dec.Decode(&user); err != nil {
			log.Printf("error decoding Say message: %v", err)
			return
		}
		if err := dec.Decode(&message); err != nil {
			log.Printf("error decoding Say message: %v", err)
			return
		}
		Say(user, message)
	case MsgQuit:
		var user string
		if err := dec.Decode(&user); err != nil {
			log.Printf("error decoding Quit message: %v", err)
			return
		}
		Quit(user)
	case MsgShutdown:
		Shutdown()
	default:
		log.Printf("unknown message type: %d", msgType)
	}
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
	log.Print("tell <user> message: messages user directly" +
		"say: says message to all users" +
		"list: Shows list of users" +
		"quit: Quits proram" +
		"shutdown: Shutdown server")
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

func waitAndCheck(user string, stop programQuit) {
	for stop.Quit == false {
		log.Print(CheckMessages(user))
		time.Sleep(1000)
	}
}

func client(serverAddress, user string) {
	stop := programQuit{Quit: false}
	//connect to server
	Register(user)
	go waitAndCheck(user, stop)
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
			log.Print(List())
		case "tell":
			//recreate tell message
			originalMessage := ""
			for i, word := range s {
				if i >= 2 {
					originalMessage += word
					if i != len(s) - 1 {
						originalMessage += " "
					}
				}
			}
			Tell(user, s[1], originalMessage)
		case "say":
			//recreate say message
			originalMessage := ""
			for i, word := range s {
				if i >= 1 {
					originalMessage += word
					if i != len(s) - 1 {
						originalMessage += " "
					}
				}
			}
			Say(user, originalMessage)
		case "quit":
			stop.Quit = true
			Quit(user)
		case "shutdown":
			stop.Quit = true
			Shutdown()
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
