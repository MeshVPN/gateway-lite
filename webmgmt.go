package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- user: register / profile ----------

func registerUser(c *gin.Context) {
	data, _ := c.GetRawData()
	req := ProfileInfo{}
	if err := json.Unmarshal(data, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 1, "detail": "wrong request"})
		return
	}
	if len(req.User) < MinNodeNameLen || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": 2, "detail": "user must >=8 chars and password required"})
		return
	}
	if ok, reason := checkPasswordStrength(req.Password); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": 5, "detail": reason})
		return
	}
	if gStore.getUser(req.User) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 3, "detail": "user already exists"})
		return
	}
	token := nodeNameToID(req.User + req.Password)
	u := &StoreUser{
		User:     req.User,
		Password: req.Password,
		Token:    token,
		Email:    req.Email,
		Phone:    req.Phone,
		Addtime:  time.Now().Format("2006-01-02 15:04:05"),
	}
	if err := gStore.addUser(u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 4, "detail": err.Error()})
		return
	}
	gLog.Println(LvINFO, "register user:", req.User)
	c.JSON(http.StatusOK, gin.H{
		"error":     0,
		"nodeToken": strconv.FormatUint(token, 10),
	})
}

func getProfile(c *gin.Context) {
	user := ctxUser(c)
	u := gStore.getUser(user)
	if u == nil {
		c.JSON(http.StatusOK, gin.H{"error": 1, "detail": "user not found"})
		return
	}
	c.JSON(http.StatusOK, ProfileInfo{
		User:    u.User,
		Token:   strconv.FormatUint(u.Token, 10),
		Email:   u.Email,
		Phone:   u.Phone,
		Addtime: u.Addtime,
	})
}

func updateProfile(c *gin.Context) {
	user := ctxUser(c)
	data, _ := c.GetRawData()
	req := ProfileInfo{}
	if err := json.Unmarshal(data, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 1, "detail": "wrong request"})
		return
	}
	if err := gStore.updateUserProfile(user, req.Email, req.Phone); err != nil {
		c.JSON(http.StatusOK, gin.H{"error": 2, "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"error": 0})
}

// ---------- device: edit node(rename/bandwidth) ----------

func editNode(c *gin.Context) {
	nodeName := c.Param("name")
	uuid := nodeNameToID(nodeName)
	data, _ := c.GetRawData()
	req := EditNode{}
	if err := json.Unmarshal(data, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 1, "detail": "wrong request"})
		return
	}
	gWSSessionMgr.allSessionsMtx.RLock()
	sess, ok := gWSSessionMgr.allSessions[uuid]
	gWSSessionMgr.allSessionsMtx.RUnlock()
	if ok {
		sess.write(MsgPush, MsgPushEditNode, req)
	}
	if req.NewName != "" && req.NewName != nodeName {
		gStore.renameDevice(nodeName, req.NewName)
	}
	c.JSON(http.StatusOK, gin.H{"error": 0})
}

// ---------- sdwan: network management ----------

func listNetworks(c *gin.Context) {
	user := ctxUser(c)
	c.JSON(http.StatusOK, gin.H{
		"error":    0,
		"networks": gStore.listNetworksByUser(user),
	})
}

type saveNetworkReq struct {
	ID            uint64 `json:"id,omitempty"`
	Name          string `json:"name"`
	Gateway       string `json:"gateway"` // e.g. 10.2.3.254/24
	Mode          string `json:"mode,omitempty"`
	CentralNode   string `json:"centralNode,omitempty"`
	ForceRelay    int32  `json:"forceRelay,omitempty"`
	PunchPriority int32  `json:"punchPriority,omitempty"`
	Enable        int32  `json:"enable,omitempty"`
	TunnelNum     int32  `json:"tunnelNum,omitempty"`
	Mtu           int32  `json:"mtu,omitempty"`
}

func saveNetwork(c *gin.Context) {
	user := ctxUser(c)
	data, _ := c.GetRawData()
	req := saveNetworkReq{}
	if err := json.Unmarshal(data, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 1, "detail": "wrong request"})
		return
	}
	if req.Gateway == "" || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": 2, "detail": "name and gateway required"})
		return
	}
	var n *StoreNetwork
	if req.ID != 0 {
		n = gStore.getNetwork(req.ID)
		if n == nil || n.User != user {
			c.JSON(http.StatusOK, gin.H{"error": 3, "detail": "network not found"})
			return
		}
	} else {
		n = &StoreNetwork{
			ID:      nodeNameToID(user + req.Name + strconv.FormatInt(time.Now().UnixNano(), 10)),
			User:    user,
			Members: []*StoreNetworkNode{},
		}
	}
	n.Name = req.Name
	n.Gateway = req.Gateway
	n.Mode = req.Mode
	if n.Mode == "" {
		n.Mode = "fullmesh"
	}
	n.CentralNode = req.CentralNode
	n.ForceRelay = req.ForceRelay
	n.PunchPriority = req.PunchPriority
	n.Enable = req.Enable
	n.TunnelNum = req.TunnelNum
	n.Mtu = req.Mtu
	gStore.saveNetwork(n)
	notifyNetworkMembers(n)
	c.JSON(http.StatusOK, gin.H{"error": 0, "id": strconv.FormatUint(n.ID, 10)})
}

func deleteNetwork(c *gin.Context) {
	user := ctxUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	n := gStore.getNetwork(id)
	if n == nil || n.User != user {
		c.JSON(http.StatusOK, gin.H{"error": 1, "detail": "network not found"})
		return
	}
	gStore.deleteNetwork(id)
	notifyNetworkMembers(n) // members refresh -> leave network
	c.JSON(http.StatusOK, gin.H{"error": 0})
}

type memberReq struct {
	Name     string `json:"name"`
	IP       string `json:"ip,omitempty"` // optional, auto-allocated if empty
	Resource string `json:"resource,omitempty"`
	Enable   int32  `json:"enable,omitempty"`
}

func addNetworkMember(c *gin.Context) {
	user := ctxUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	data, _ := c.GetRawData()
	req := memberReq{}
	if err := json.Unmarshal(data, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 1, "detail": "wrong request"})
		return
	}
	if len(req.Name) < MinNodeNameLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": 2, "detail": "node name too short"})
		return
	}
	n := gStore.getNetwork(id)
	if n == nil || n.User != user {
		c.JSON(http.StatusOK, gin.H{"error": 3, "detail": "network not found"})
		return
	}
	// update existing or add new
	var member *StoreNetworkNode
	for _, m := range n.Members {
		if m.Name == req.Name {
			member = m
			break
		}
	}
	if member == nil {
		ip := req.IP
		if ip == "" {
			allocated, err := allocVirtualIP(n)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"error": 4, "detail": err.Error()})
				return
			}
			ip = allocated
		}
		member = &StoreNetworkNode{Name: req.Name, IP: ip}
		n.Members = append(n.Members, member)
	}
	if req.IP != "" {
		member.IP = req.IP
	}
	member.Resource = req.Resource
	member.Enable = req.Enable
	gStore.saveNetwork(n)
	notifyNetworkMembers(n)
	c.JSON(http.StatusOK, gin.H{"error": 0, "ip": member.IP})
}

func delNetworkMember(c *gin.Context) {
	user := ctxUser(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	data, _ := c.GetRawData()
	req := memberReq{}
	if err := json.Unmarshal(data, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": 1, "detail": "wrong request"})
		return
	}
	n := gStore.getNetwork(id)
	if n == nil || n.User != user {
		c.JSON(http.StatusOK, gin.H{"error": 2, "detail": "network not found"})
		return
	}
	removed := req.Name
	members := make([]*StoreNetworkNode, 0, len(n.Members))
	for _, m := range n.Members {
		if m.Name == removed {
			continue
		}
		members = append(members, m)
	}
	n.Members = members
	gStore.saveNetwork(n)
	notifyNetworkMembers(n)
	// also notify the removed node to refresh(leave network)
	pushSDWanRefresh(removed)
	c.JSON(http.StatusOK, gin.H{"error": 0})
}

// notifyNetworkMembers pushes MsgPushSDWanRefresh to all online members so they
// re-request the latest SDWAN config.
func notifyNetworkMembers(n *StoreNetwork) {
	for _, m := range n.Members {
		pushSDWanRefresh(m.Name)
	}
}

func pushSDWanRefresh(node string) {
	uuid := nodeNameToID(node)
	gWSSessionMgr.allSessionsMtx.RLock()
	sess, ok := gWSSessionMgr.allSessions[uuid]
	gWSSessionMgr.allSessionsMtx.RUnlock()
	if !ok {
		return
	}
	// the client always reads a PushHeader(16B) for every MsgPush before
	// dispatching, so include one even though the refresh has no body.
	pushHead := PushHeader{From: 0, To: uuid}
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, pushHead)
	msg := append(encodeHeader(MsgPush, MsgPushSDWanRefresh, uint32(buf.Len())), buf.Bytes()...)
	sess.writeBuff(msg)
}
