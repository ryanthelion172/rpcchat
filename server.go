package main
const (
        MsgRegister = iota
        MsgList
        MsgCheckMessages
        MsgTell
        MsgSay
        MsgQuit
        MsgShutdown
)

var mutex sync.Mutex
var messages map[string][]string
var shutdown chan struct{}

func server(listenAddress string) {
        shutdown = make(chan struct{})
        messages = make(map[string][]string)

        // set up network listen and accept loop here
        // to receive and dispatch RPC requests
        // ...

        // wait for a shutdown request
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
