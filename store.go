package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StoreUser is a persisted user record. In single-user mode there is one record
// derived from OPENP2P_USER/OPENP2P_PASSWORD; register API can add more.
type StoreUser struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Token    uint64 `json:"token"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Addtime  string `json:"addtime,omitempty"`
}

// StoreDevice is a persisted device record, kept even when the device is offline.
type StoreDevice struct {
	Name       string `json:"name"`
	User       string `json:"user"`
	OS         string `json:"os,omitempty"`
	MAC        string `json:"mac,omitempty"`
	LanIP      string `json:"lanip,omitempty"`
	IPv4       string `json:"ipv4,omitempty"`
	IPv6       string `json:"ipv6,omitempty"`
	NatType    int    `json:"natType,omitempty"`
	Bandwidth  int    `json:"bandwidth,omitempty"`
	Version    string `json:"version,omitempty"`
	Remark     string `json:"remark,omitempty"`
	Addtime    string `json:"addtime,omitempty"`
	Activetime string `json:"activetime,omitempty"`
}

// StoreNetwork is a persisted SDWAN virtual network.
type StoreNetwork struct {
	ID            uint64              `json:"id"`
	User          string              `json:"user"`
	Name          string              `json:"name"`
	Gateway       string              `json:"gateway"` // e.g. 10.2.3.254/24
	Mode          string              `json:"mode"`    // fullmesh / central
	CentralNode   string              `json:"centralNode,omitempty"`
	ForceRelay    int32               `json:"forceRelay,omitempty"`
	PunchPriority int32               `json:"punchPriority,omitempty"`
	Enable        int32               `json:"enable"`
	TunnelNum     int32               `json:"tunnelNum,omitempty"`
	Mtu           int32               `json:"mtu,omitempty"`
	Members       []*StoreNetworkNode `json:"members"`
}

// StoreNetworkNode is a member of a virtual network.
type StoreNetworkNode struct {
	Name     string `json:"name"`
	IP       string `json:"ip"` // virtual ip, e.g. 10.2.3.1
	Resource string `json:"resource,omitempty"`
	Enable   int32  `json:"enable"`
}

type storeData struct {
	Users    map[string]*StoreUser    `json:"users"`
	Devices  map[string]*StoreDevice  `json:"devices"`  // key: node name
	Networks map[uint64]*StoreNetwork `json:"networks"` // key: network id
}

type Store struct {
	mtx  sync.RWMutex
	path string
	data storeData
}

var gStore *Store

func newStore(dir string) *Store {
	s := &Store{
		path: filepath.Join(dir, "gateway-data.json"),
		data: storeData{
			Users:    make(map[string]*StoreUser),
			Devices:  make(map[string]*StoreDevice),
			Networks: make(map[uint64]*StoreNetwork),
		},
	}
	s.load()
	return s
}

func (s *Store) load() {
	buf, err := os.ReadFile(s.path)
	if err != nil {
		return // first run, no file yet
	}
	d := storeData{}
	if err := json.Unmarshal(buf, &d); err != nil {
		gLog.Printf(LvERROR, "store load error:%s", err)
		return
	}
	if d.Users != nil {
		s.data.Users = d.Users
	}
	if d.Devices != nil {
		s.data.Devices = d.Devices
	}
	if d.Networks != nil {
		s.data.Networks = d.Networks
	}
	gLog.Printf(LvINFO, "store loaded: %d users, %d devices, %d networks", len(s.data.Users), len(s.data.Devices), len(s.data.Networks))
}

// save assumes the caller holds the write lock.
func (s *Store) save() {
	buf, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		gLog.Printf(LvERROR, "store marshal error:%s", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0600); err != nil {
		gLog.Printf(LvERROR, "store write error:%s", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		gLog.Printf(LvERROR, "store rename error:%s", err)
	}
}

// ---------- user ----------

func (s *Store) getUser(user string) *StoreUser {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.data.Users[user]
}

func (s *Store) getUserByToken(token uint64) *StoreUser {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	for _, u := range s.data.Users {
		if u.Token == token {
			return u
		}
	}
	return nil
}

func (s *Store) addUser(u *StoreUser) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if _, ok := s.data.Users[u.User]; ok {
		return fmt.Errorf("user already exists")
	}
	if u.Addtime == "" {
		u.Addtime = time.Now().Format("2006-01-02 15:04:05")
	}
	s.data.Users[u.User] = u
	s.save()
	return nil
}

func (s *Store) updateUserProfile(user, email, phone string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	u, ok := s.data.Users[user]
	if !ok {
		return fmt.Errorf("user not found")
	}
	u.Email = email
	u.Phone = phone
	s.save()
	return nil
}

// ensureUser makes sure the built-in single-user record exists.
func (s *Store) ensureUser(user, password string, token uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if u, ok := s.data.Users[user]; ok {
		u.Password = password
		u.Token = token
		s.save()
		return
	}
	s.data.Users[user] = &StoreUser{
		User:     user,
		Password: password,
		Token:    token,
		Addtime:  time.Now().Format("2006-01-02 15:04:05"),
	}
	s.save()
}

// ---------- device ----------

func (s *Store) upsertDevice(d *StoreDevice) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	old, ok := s.data.Devices[d.Name]
	if ok {
		if d.Addtime == "" {
			d.Addtime = old.Addtime
		}
		if d.Remark == "" {
			d.Remark = old.Remark
		}
	} else if d.Addtime == "" {
		d.Addtime = time.Now().Format("2006-01-02 15:04:05")
	}
	s.data.Devices[d.Name] = d
	s.save()
}

func (s *Store) listDevicesByUser(user string) []*StoreDevice {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	out := []*StoreDevice{}
	for _, d := range s.data.Devices {
		if d.User == user {
			out = append(out, d)
		}
	}
	return out
}

func (s *Store) renameDevice(oldName, newName string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	d, ok := s.data.Devices[oldName]
	if !ok {
		return
	}
	delete(s.data.Devices, oldName)
	d.Name = newName
	s.data.Devices[newName] = d
	s.save()
}

// ---------- network ----------

func (s *Store) listNetworksByUser(user string) []*StoreNetwork {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	out := []*StoreNetwork{}
	for _, n := range s.data.Networks {
		if n.User == user {
			out = append(out, n)
		}
	}
	return out
}

func (s *Store) getNetwork(id uint64) *StoreNetwork {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.data.Networks[id]
}

// findNetworkByNode returns the enabled network that contains the given node.
func (s *Store) findNetworkByNode(user, node string) *StoreNetwork {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	for _, n := range s.data.Networks {
		if n.User != user || n.Enable == 0 {
			continue
		}
		for _, m := range n.Members {
			if m.Name == node {
				return n
			}
		}
	}
	return nil
}

func (s *Store) saveNetwork(n *StoreNetwork) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.data.Networks[n.ID] = n
	s.save()
}

func (s *Store) deleteNetwork(id uint64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.data.Networks, id)
	s.save()
}

// allocVirtualIP picks the next free virtual ip in the network's subnet,
// skipping the gateway address. assumes caller holds no lock (uses snapshot).
func allocVirtualIP(n *StoreNetwork) (string, error) {
	gw, ipnet, err := net.ParseCIDR(n.Gateway)
	if err != nil {
		return "", fmt.Errorf("invalid gateway %s:%s", n.Gateway, err)
	}
	used := map[string]bool{gw.String(): true}
	for _, m := range n.Members {
		used[m.IP] = true
	}
	ip := ipnet.IP.Mask(ipnet.Mask).To4()
	if ip == nil {
		return "", fmt.Errorf("only ipv4 subnet supported")
	}
	for i := 0; i < (1 << 24); i++ {
		incIP(ip)
		if !ipnet.Contains(ip) {
			break
		}
		// skip network and broadcast-ish boundaries naturally via Contains
		cand := ip.String()
		if !used[cand] {
			return cand, nil
		}
	}
	return "", fmt.Errorf("no free virtual ip in %s", n.Gateway)
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
