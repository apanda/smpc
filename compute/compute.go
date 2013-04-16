package main
import (
        zmq "github.com/apanda/go-zmq"
        "fmt"
        "flag"
        "os"
        "os/signal"
        sproto "github.com/apanda/smpc/proto"
        "sync"
        )
type RequestStepPair struct {
    Request int64
    Step int32
}

func MakeRequestStep (req int64, step int32) (ret *RequestStepPair) {
    ret = &RequestStepPair{}
    ret.Request = req
    ret.Step = step
    return ret
}

type ComputePeerState struct {
    SubSock *zmq.Socket
    CoordSock *zmq.Socket
    PeerInSock *zmq.Socket
    PeerOutSocks map[int] *zmq.Socket
    SubChannel *zmq.Channels
    CoordChannel *zmq.Channels
    PeerInChannel *zmq.Channels
    PeerOutChannels map[int] *zmq.Channels
    Shares map[string] int64
    HasShare map[string] bool
    ShareLock sync.RWMutex
    Client int
    NumClients int
    ChannelMap map[RequestStepPair] chan *sproto.IntermediateData 
    ChannelLock sync.Mutex
    SquelchTraffic map[RequestStepPair] bool
}

const INITIAL_MAP_CAPACITY int = 1000
const INITIAL_CHANNEL_CAPACITY int = 100

func MakeComputePeerState (client int, numClients int) (*ComputePeerState) {
    state := &ComputePeerState{}
    state.Client = client
    state.Shares = make(map[string] int64, INITIAL_MAP_CAPACITY)
    state.HasShare = make(map[string] bool, INITIAL_MAP_CAPACITY)
    state.NumClients = numClients
    state.PeerOutSocks = make(map[int] *zmq.Socket, numClients)
    state.PeerOutChannels = make(map[int] *zmq.Channels, numClients)
    state.ChannelMap = make(map[RequestStepPair] chan *sproto.IntermediateData, INITIAL_MAP_CAPACITY)
    state.SquelchTraffic = make(map[RequestStepPair] bool, INITIAL_MAP_CAPACITY)
    return state
}

const BUFFER_SIZE int = 10

func (state *ComputePeerState) UnregisterChannelForRequest (request RequestStepPair) {
    state.ChannelLock.Lock()
    defer state.ChannelLock.Unlock()
    state.SquelchTraffic[request] = true
    delete(state.ChannelMap, request)
}

func (state *ComputePeerState) ChannelForRequest (request RequestStepPair) (chan *sproto.IntermediateData) {
    state.ChannelLock.Lock()
    defer state.ChannelLock.Unlock()
    ch := state.ChannelMap[request]
    if ch == nil {
        state.ChannelMap[request] = make(chan *sproto.IntermediateData, INITIAL_CHANNEL_CAPACITY)
        ch = state.ChannelMap[request]
    }
    return ch
}

func (state *ComputePeerState) ReceiveFromPeers () {
    for {
        //fmt.Printf("Core is now waiting for messages\n")
        select {
            case msg := <- state.PeerInChannel.In():
                fmt.Println("Message on peer channel")
                //fmt.Printf("Received message at core\n")
                intermediate := MsgToIntermediate(msg)
                //fmt.Printf("Core received %d->%d request=%d\n", *intermediate.Client, state.Client, *intermediate.RequestCode)
                if intermediate == nil {
                    panic ("Could not read intermediate message")
                }
                key := MakeRequestStep(*intermediate.RequestCode, *intermediate.Step)
                if !state.SquelchTraffic[*key] {
                    ch := state.ChannelForRequest(*key)
                    ch <- intermediate
                }
        }
    }
}

func (state *ComputePeerState) SharesGet (share string) (int64, bool) {
    state.ShareLock.RLock()
    defer state.ShareLock.RUnlock()
    val := state.Shares[share]
    has := state.HasShare[share]
    return val, has
}

func (state *ComputePeerState) SharesSet (share string, value int64) {
    //fmt.Println("SharesSet called, locking")
    state.ShareLock.Lock()
    defer state.ShareLock.Unlock()
    //fmt.Println("SharesSet called, locked")
    state.Shares[share] = value
    //fmt.Printf("Set %v to %v\n", share, value)
    state.HasShare[share] = true
    //fmt.Printf("Set %v to %v\n", share, true)
}

func (state *ComputePeerState) SharesDelete (share string) {
    //fmt.Println("SharesDelete called, locking")
    state.ShareLock.Lock()
    defer state.ShareLock.Unlock()
    //fmt.Println("ShareDelete called, locked")
    delete(state.Shares, share)
    delete(state.HasShare, share)
    //fmt.Printf("Deleted")
}

func (state *ComputePeerState) DispatchAction (action *sproto.Action, r chan<- [][]byte) {
    //fmt.Println("Dispatching action")
    var resp *sproto.Response
    switch *action.Action {
        case sproto.Action_Set:
            //fmt.Println("Dispatching SET")
            resp = state.SetValue(action)
        case sproto.Action_Add:
            //fmt.Println("Dispatching ADD")
            resp = state.Add(action)
        case sproto.Action_Retrieve:
            //fmt.Println("Retrieving value")
            resp = state.GetValue(action)
        case sproto.Action_Mul:
            //fmt.Println("Dispatching mul")
            resp = state.Mul(action)
            //fmt.Println("Return from mul")
        case sproto.Action_Cmp:
            //fmt.Println("Dispatching CMP")
            resp = state.Cmp(action)
            //fmt.Println("Return from cmp")
        case sproto.Action_Neq:
            //fmt.Println("Dispatching NEQ")
            resp = state.Neq(action)
            //fmt.Println("Return from NEQ")
        case sproto.Action_Eqz:
            //fmt.Println("Dispatching EQZ")
            resp = state.Eqz(action)
            //fmt.Println("Returning from EQZ")
        case sproto.Action_Neqz:
            //fmt.Println("Dispatching NEQZ")
            resp = state.Neqz(action)
            //fmt.Println("Returning from NEQZ")
        case sproto.Action_Del:
            //fmt.Println("Dispatching DEL")
            resp = state.RemoveValue(action)
            //fmt.Println("Return from DEL")
        case sproto.Action_OneSub:
            //fmt.Println("Dispatching 1SUB")
            resp = state.OneSub(action)
            //fmt.Println("Return from 1SUB")
        case sproto.Action_CmpConst:
            //fmt.Println("Dispatching CmpConst")
            resp = state.CmpConst(action)
            //fmt.Println("Returning from CmpConst")
        case sproto.Action_NeqConst:
            //fmt.Println("Dispatching NeqConst")
            resp = state.NeqConst(action)
            //fmt.Println("Returning from NeqConst")
        case sproto.Action_MulConst:
            //fmt.Println("Dispatching MulConst")
            resp = state.MulConst(action)
            //fmt.Println("Returning from MulConst")
        default:
            //fmt.Println("Unimplemented action")
            resp = state.DefaultAction(action)
    }
    respB := ResponseToMsg(resp)
    if resp == nil {
        panic ("Malformed response")
    }
    r <- respB
}

func (state *ComputePeerState) ActionMsg (msg [][]byte) {
    //fmt.Println("Received message from coordination channel")
    action := MsgToAction(msg)
    //fmt.Println("Converted to action")
    if action == nil {
        panic ("Malformed action")
    }
    go state.DispatchAction(action, state.CoordChannel.Out())
}

func EventLoop (config *string, client int, q chan int) {
    configStruct := ParseConfig(config, q) 
    state := MakeComputePeerState(client, len(configStruct.Clients)) 
    // Create the 0MQ context
    ctx, err := zmq.NewContext()
    if err != nil {
        //fmt.Println("Error creating 0mq context: ", err)
        q <- 1
    }
    // Establish the PUB-SUB connection that will be used to direct all the computation clusters
    state.SubSock, err = ctx.Socket(zmq.Sub)
    if err != nil {
        //fmt.Println("Error creating PUB socket: ", err)
        q <- 1
    }
    err = state.SubSock.Connect(configStruct.PubAddress)
    if err != nil {
        //fmt.Println("Error binding PUB socket: ", err)
        q <- 1
    }
    // Establish coordination socket
    state.CoordSock, err = ctx.Socket(zmq.Dealer)
    if err != nil {
        //fmt.Println("Error creating Dealer socket: ", err)
        q <- 1
    }
    err = state.CoordSock.Connect(configStruct.ControlAddress)
    if err != nil {
        //fmt.Println("Error connecting  ", err)
        q <- 1
    }
    state.PeerInSock, err = ctx.Socket(zmq.Router)
    if err != nil {
        //fmt.Println("Error creating peer router socket: ", err)
        q <- 1
    }
    err = state.PeerInSock.Bind(configStruct.Clients[client]) // Set up something to listen to peers
    if err != nil {
        //fmt.Println("Error binding peer router socket")
        q <- 1
    }
    for index, value := range configStruct.Clients {
        if index != client {
            state.PeerOutSocks[index], err= ctx.Socket(zmq.Dealer)
            if err != nil {
                //fmt.Println("Error creating dealer socket: ", err)
                q <- 1
                return
            }
            err  = state.PeerOutSocks[index].Connect(value)
            if err != nil {
                //fmt.Println("Error connection ", err)
                q <- 1
                return
            }
            state.PeerOutChannels[index] = state.PeerOutSocks[index].ChannelsBuffer(BUFFER_SIZE)
        }
    }
    state.PeerInChannel = state.PeerInSock.Channels()
    go state.ReceiveFromPeers()
    state.Sync(q)
    state.IntermediateSync(q)
    state.SubSock.Subscribe([]byte("CMD"))
    //fmt.Println("Receiving")
    // We cannot create channels before finalizing the set of subscriptions, since sockets are
    // not thread safe. Hence first sync, then get channels
    state.SubChannel = state.SubSock.ChannelsBuffer(BUFFER_SIZE)
    state.CoordChannel = state.CoordSock.ChannelsBuffer(BUFFER_SIZE)
    defer func() {
        state.SubSock.Close()
        state.CoordSock.Close()
        ctx.Close()
        //fmt.Println("Closed socket")
    }()
    for true {
        //fmt.Println("Starting to wait")
        select {
            case msg := <- state.SubChannel.In():
                fmt.Println("Message on SubChannel")
                state.ActionMsg(msg) 
            case msg := <- state.CoordChannel.In():
                fmt.Println("Message on CoordChannel")
                state.ActionMsg(msg)
            case err = <- state.SubChannel.Errors():
                //fmt.Println("Error in SubChannel", err)
                q <- 1
                return
            case err = <- state.CoordChannel.Errors():
                //fmt.Println("Error in CoordChannel", err)
                q <- 1
                return
        }
    }
    q <- 0
}

func main() {
    // Start up by setting up a flag for the configuration file
    config := flag.String("config", "conf", "Configuration file")
    client := flag.Int("peer", 0, "Input peer")
    flag.Parse()
    os_channel := make(chan os.Signal)
    signal.Notify(os_channel)
    end_channel := make(chan int)
    go EventLoop(config, *client, end_channel)
    var status = 0
    select {
        case <- os_channel:
        case status = <- end_channel: 
    }
    // <-signal_channel
    os.Exit(status)
}

