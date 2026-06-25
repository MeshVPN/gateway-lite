package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

type UserInfo struct {
	Password string
	Token    string
}

type TokenInfo struct {
	User    string
	Expired time.Time
}

type deviceInfo struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	IP         string `json:"ip,omitempty"`
	IPv6       string `json:"ipv6,omitempty"`
	NatType    string `json:"natType,omitempty"`
	Bandwidth  string `json:"bandwidth,omitempty"`
	LanIP      string `json:"lanip,omitempty"`
	MAC        string `json:"mac,omitempty"`
	OS         string `json:"os,omitempty"`
	IsActive   int    `json:"isActive,omitempty"`
	Version    string `json:"version,omitempty"`
	Remark     string `json:"remark,omitempty"`
	Removed    int    `json:"removed,omitempty"`
	Activetime string `json:"activetime,omitempty"`
	Addtime    string `json:"addtime,omitempty"`
	IsUpdate   bool   `json:"isUpdate,omitempty"`
}

type deviceList struct {
	Nodes     []deviceInfo `json:"nodes" binding:"required"`
	LatestVer string       `json:"latestVer,omitempty"`
}

var JWTSecret string

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// just token not jwt
		auth, ok := c.Request.Header["Authorization"]
		if !ok {
			c.String(http.StatusUnauthorized, "")
			c.Abort()
			return
		}
		token, err := jwt.ParseWithClaims(auth[0], &OpenP2PClaim{}, func(token *jwt.Token) (interface{}, error) {
			// Don't forget to validate the alg is what you expect:
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(JWTSecret), nil
		})
		if err != nil {
			gLog.Println(LvERROR, "Parse token error:", err)
			c.String(http.StatusUnauthorized, "")
			c.Abort()
			return
		}
		claims, ok := token.Claims.(*OpenP2PClaim)
		if ok && token.Valid {
			fmt.Println(claims)
			if claims.StandardClaims.ExpiresAt < time.Now().Unix() {
				c.String(http.StatusUnauthorized, "")
				c.Abort()
				return
			}
			c.Set("user", claims.User)
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Next()
	}
}

func ctxUser(c *gin.Context) string {
	if u, ok := c.Get("user"); ok {
		if s, ok := u.(string); ok && s != "" {
			return s
		}
	}
	return gUser
}

func runWeb() {
	router := gin.Default()
	router.Use(CORSMiddleware())
	router.GET("/", serveWebUI)
	router.GET("/index.html", serveWebUI)
	router.GET("/api/v1/update", updateInfo)
	router.GET("/api/v1/devices", AuthMiddleware(), listDevices)
	router.GET("/api/v1/device/:name/restart", AuthMiddleware(), restartDevice)
	user := router.Group("/api/v1/user")
	user.POST("/login", webLogin)
	user.POST("/register", registerUser)
	user.GET("/profile", AuthMiddleware(), getProfile)
	user.POST("/profile", AuthMiddleware(), updateProfile)
	device := router.Group("/api/v1/device")
	device.Use(AuthMiddleware())
	device.GET("/:name/apps", listApps)
	device.POST("/:name/app", editApp)
	device.POST("/:name/switchapp", switchApp)
	device.POST("/:name/editnode", editNode)
	device.GET("/:name/log", getDeviceLog)
	sdwan := router.Group("/api/v1/sdwan")
	sdwan.Use(AuthMiddleware())
	sdwan.GET("/networks", listNetworks)
	sdwan.POST("/network", saveNetwork)
	sdwan.POST("/network/:id/delete", deleteNetwork)
	sdwan.POST("/network/:id/member", addNetworkMember)
	sdwan.POST("/network/:id/member/delete", delNetworkMember)
	router.RunTLS(":10008", "api.crt", "api.key")
	// router.Run(":10008")
}

// updateInfo is the client auto-update endpoint. self-hosting gateway does not
// host binaries, so it reports "already latest"(Error:0, empty Url) to keep the
// client update check from erroring.
func updateInfo(c *gin.Context) {
	c.JSON(http.StatusOK, UpdateInfo{Error: 0})
}

func webLogin(c *gin.Context) {
	data, _ := c.GetRawData()
	req := ProfileInfo{}
	err := json.Unmarshal(data, &req)
	if err != nil {
		log.Println("wrong loginReq")
		c.JSON(http.StatusBadRequest, gin.H{"error": 1, "detail": "请求格式错误"})
		return
	}
	gLog.Println(LvINFO, "webLogin:", req.User)
	// brute-force protection: throttle by client IP + username
	guardKey := c.ClientIP() + "|" + req.User
	if ok, wait := gLoginGuard.allowed(guardKey); !ok {
		gLog.Println(LvWARN, "login locked:", guardKey)
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":  429,
			"detail": fmt.Sprintf("登录失败次数过多，请%d秒后重试", wait),
		})
		return
	}
	// authenticate against the store(built-in user is also in the store)
	// constant-time password compare to avoid timing attacks; always compare
	// even when the user does not exist to keep timing uniform.
	u := gStore.getUser(req.User)
	stored := ""
	if u != nil {
		stored = u.Password
	}
	if u == nil || !constantTimeEqual(stored, req.Password) {
		gLoginGuard.onFailure(guardKey)
		c.JSON(http.StatusUnauthorized, gin.H{"error": 401, "detail": "用户名或密码错误"})
		log.Println("authorize error:")
		return
	}
	gLoginGuard.onSuccess(guardKey)
	nodeToken := u.Token
	// new token
	claim := OpenP2PClaim{
		User:         req.User,
		InstallToken: fmt.Sprintf("%d", nodeToken),
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().AddDate(0, 0, 1).Unix(),
			// ExpiresAt: time.Now().Add(time.Second * 60).Unix(),  // test
			Issuer:   "openp2p.cn",
			IssuedAt: time.Now().Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString([]byte(JWTSecret))
	if err != nil {
		fmt.Println(tokenString, JWTSecret, err)
		return
	}

	log.Println("authorize ok:")
	c.JSON(http.StatusOK, gin.H{
		"token":     tokenString,
		"nodeToken": fmt.Sprintf("%d", nodeToken),
		"error":     0,
	})
}

func listDevices(c *gin.Context) {
	user := ctxUser(c)
	nodes := deviceList{}
	online := make(map[string]bool)
	// online devices from memory
	gWSSessionMgr.allSessionsMtx.RLock()
	for _, sess := range gWSSessionMgr.allSessions {
		if sess.user != user {
			continue
		}
		online[sess.node] = true
		data := deviceInfo{}
		data.Name = sess.node
		data.NatType = fmt.Sprintf("%d", sess.natType)
		data.Bandwidth = fmt.Sprintf("%d", sess.shareBandWidth)
		data.IP = sess.IPv4
		data.IPv6 = sess.IPv6
		data.LanIP = sess.lanIP
		data.MAC = sess.mac
		data.OS = sess.os
		data.Version = sess.version
		data.Activetime = sess.activeTime.Local().String()
		data.IsActive = 1
		data.IsUpdate = true
		data.ID = fmt.Sprintf("%d", nodeNameToID(data.Name))
		nodes.Nodes = append(nodes.Nodes, data)
	}
	gWSSessionMgr.allSessionsMtx.RUnlock()
	// offline devices from store
	for _, d := range gStore.listDevicesByUser(user) {
		if online[d.Name] {
			continue
		}
		data := deviceInfo{}
		data.Name = d.Name
		data.NatType = fmt.Sprintf("%d", d.NatType)
		data.Bandwidth = fmt.Sprintf("%d", d.Bandwidth)
		data.IP = d.IPv4
		data.IPv6 = d.IPv6
		data.LanIP = d.LanIP
		data.MAC = d.MAC
		data.OS = d.OS
		data.Version = d.Version
		data.Remark = d.Remark
		data.Addtime = d.Addtime
		data.Activetime = d.Activetime
		data.IsActive = 0
		data.IsUpdate = false
		data.ID = fmt.Sprintf("%d", nodeNameToID(data.Name))
		nodes.Nodes = append(nodes.Nodes, data)
	}
	log.Println("get devices:", nodes)
	c.JSON(http.StatusOK, nodes)
}

func listApps(c *gin.Context) {

	nodeName := c.Param("name")
	uuid := nodeNameToID(nodeName)
	gLog.Println(LvINFO, nodeName, " update")
	gWSSessionMgr.allSessionsMtx.Lock()
	sess, ok := gWSSessionMgr.allSessions[uuid]
	gWSSessionMgr.allSessionsMtx.Unlock()

	if !ok {
		gLog.Printf(LvERROR, "listTunnel %d error: peer offline", uuid)
		c.JSON(http.StatusOK, gin.H{"error": 1, "detail": "device offline"})
		return
	}
	sess.write(MsgPush, MsgPushReportApps, nil)
	// TODO verify token
	// wait for the channel at most 5 seconds
	select {
	case msg := <-sess.rspCh:
		c.String(http.StatusOK, "%s", msg)
	case <-time.After(ClientAPITimeout):
		// Timed out after 5 seconds!
		log.Printf("listTunnel %d timeout.", uuid)
		c.JSON(http.StatusNotFound, gin.H{"error": 9, "detail": "timeout"})
	}
}

// getDeviceLog requests the realtime running log of an online device. The
// client reads the log file from its disk and pushes back the content. The
// frontend polls this endpoint(with offset) to implement realtime tailing.
func getDeviceLog(c *gin.Context) {
	nodeName := c.Param("name")
	uuid := nodeNameToID(nodeName)
	gWSSessionMgr.allSessionsMtx.Lock()
	sess, ok := gWSSessionMgr.allSessions[uuid]
	gWSSessionMgr.allSessionsMtx.Unlock()
	if !ok {
		c.JSON(http.StatusOK, gin.H{"error": 1, "detail": "device offline"})
		return
	}
	offset, _ := strconv.ParseInt(c.Query("offset"), 10, 64)
	length, _ := strconv.ParseInt(c.Query("len"), 10, 64)
	if length <= 0 {
		length = 64 * 1024
	}
	req := ReportLogReq{
		FileName: "openp2p.log",
		Offset:   offset,
		Len:      length,
	}
	// drain any stale response before sending a new request
	select {
	case <-sess.rspCh:
	default:
	}
	sess.write(MsgPush, MsgPushReportLog, &req)
	select {
	case msg := <-sess.rspCh:
		c.Data(http.StatusOK, "application/json; charset=utf-8", msg)
	case <-time.After(ClientAPITimeout):
		c.JSON(http.StatusOK, gin.H{"error": 9, "detail": "timeout"})
	}
}

func editApp(c *gin.Context) {
	nodeName := c.Param("name")
	uuid := nodeNameToID(nodeName)
	gWSSessionMgr.allSessionsMtx.Lock()
	sess, ok := gWSSessionMgr.allSessions[uuid]
	gWSSessionMgr.allSessionsMtx.Unlock()

	if !ok {
		gLog.Printf(LvERROR, "editApp %d error: peer offline", uuid)
		c.JSON(http.StatusOK, gin.H{"error": 1, "detail": "device offline"})
		return
	}
	app := AppInfo{}
	buf, _ := c.GetRawData()
	err := json.Unmarshal(buf, &app)
	if err != nil {
		gLog.Printf(LvERROR, "wrong AppInfo:%s", err)
		c.String(http.StatusNotAcceptable, "")
		return
	}
	gLog.Println(LvINFO, "edit app:", app)
	sess.write(MsgPush, MsgPushEditApp, app)
	c.String(http.StatusOK, "")
}

func switchApp(c *gin.Context) {
	nodeName := c.Param("name")
	uuid := nodeNameToID(nodeName)
	gWSSessionMgr.allSessionsMtx.Lock()
	sess, ok := gWSSessionMgr.allSessions[uuid]
	gWSSessionMgr.allSessionsMtx.Unlock()

	if !ok {
		gLog.Printf(LvERROR, "switchApp %d error: peer offline", uuid)
		c.JSON(http.StatusOK, gin.H{"error": 1, "detail": "device offline"})
		return
	}
	app := AppInfo{}
	buf, _ := c.GetRawData()
	err := json.Unmarshal(buf, &app)
	if err != nil {
		gLog.Printf(LvERROR, "wrong AppInfo:%s", err)
		c.String(http.StatusNotAcceptable, "")
		return
	}
	gLog.Println(LvINFO, "switchApp app:", app)
	sess.write(MsgPush, MsgPushSwitchApp, app)
	c.String(http.StatusOK, "")
}

func init() {
}

type OpenP2PClaim struct {
	User         string `json:"user,omitempty"`
	InstallToken string `json:"installToken,omitempty"`
	jwt.StandardClaims
}

func restartDevice(c *gin.Context) {
	nodeName := c.Param("name")
	uuid := nodeNameToID(nodeName)
	gLog.Println(LvINFO, nodeName, " restart")
	gWSSessionMgr.allSessionsMtx.Lock()
	sess, ok := gWSSessionMgr.allSessions[uuid]
	gWSSessionMgr.allSessionsMtx.Unlock()
	if !ok {
		gLog.Printf(LvERROR, "push to %s error: peer offline", nodeName)
		c.JSON(http.StatusOK, gin.H{"error": 1, "detail": "device offline"})
		return
	}
	sess.write(MsgPush, MsgPushRestart, nil)
	c.JSON(http.StatusOK, gin.H{"error": 0})
}
