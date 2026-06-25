package main

import (
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"time"
)

var (
	gWSSessionMgr *sessionMgr
	gHandler      *msgHandler
	gToken        uint64
	gUser         string
	gPassword     string
)

func main() {
	// https://pkg.go.dev/net/http/pprof
	go func() {
		log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	}()

	gLog = NewLogger(filepath.Dir(os.Args[0]), "openp2p", LvINFO, 1*1024*1024, LogFileAndConsole)
	rand.Seed(time.Now().UnixNano())
	if err := parseParams(); err != nil {
		gLog.Println(LvERROR, err)
		return
	}
	gStore = newStore(filepath.Dir(os.Args[0]))
	gStore.ensureUser(gUser, gPassword, gToken)
	gHandler = &msgHandler{
		handlers: make(map[uint16]handlerInterface),
	}
	login := loginHandler{}
	gHandler.registerHandler(MsgLogin, &login)
	gHandler.registerHandler(MsgHeartbeat, &login)
	gHandler.registerHandler(MsgPush, &pushHandler{})
	gHandler.registerHandler(MsgRelay, &relayHandler{})
	gHandler.registerHandler(MsgReport, &reportHandler{})
	gHandler.registerHandler(MsgQuery, &queryHandler{})
	gHandler.registerHandler(MsgSDWAN, &sdwanHandler{})
	nat := natHandler{}
	gHandler.registerHandler(MsgNATDetect, &nat)
	for i := 0; i < 16; i++ {
		go gHandler.handleMessage()
	}
	initStun()
	gWSSessionMgr = NewSessionMgr()
	go gWSSessionMgr.run()
	runWeb()
	forever := make(chan bool)
	<-forever
}

func initStun() {
	go tcpServer(IfconfigPort1)
	go tcpServer(IfconfigPort2)
	// client detects NAT type by sending UDP to two different ports(NATDetectPort1/2 = 27180/27181)
	// and comparing the mapped public ports. so server must listen UDP STUN on these ports too,
	// otherwise client UDP detect always times out and falls back to TCP (UDP punch unavailable).
	stunPorts := []int{IfconfigPort1, IfconfigPort2, UDPPort1, UDPPort2}
	for _, port := range stunPorts {
		if _, err := newUDPServer(&net.UDPAddr{IP: net.IPv4zero, Port: port}); err != nil {
			gLog.Printf(LvERROR, "listen STUN UDP on %d failed:%s", port, err)
			return
		}
	}
	gLog.Printf(LvINFO, "listen STUN UDP on: %v", stunPorts)
}

func parseParams() error {
	gUser = os.Getenv("OPENP2P_USER")
	gPassword = os.Getenv("OPENP2P_PASSWORD")
	if gUser == "" || gPassword == "" {
		return ErrUserOrPwdNotSet
	} else {
		gToken = nodeNameToID(gUser + gPassword)
		gLog.Println(LvINFO, "TOKEN:", gToken)
	}
	JWTSecret = gUser + gPassword + "@openp2p.cn"
	return nil
}
