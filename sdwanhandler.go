package main

import (
	"bytes"
	"encoding/binary"
	"errors"
)

type sdwanHandler struct {
}

func (h *sdwanHandler) handleMessage(ctx *msgContext) error {
	head := openP2PHeader{}
	err := binary.Read(bytes.NewReader(ctx.msg[:openP2PHeaderSize]), binary.LittleEndian, &head)
	if err != nil {
		return err
	}
	sess, ok := ctx.sess.(*wssSession)
	if !ok {
		gLog.Println(LvERROR, "interface conversion error")
		return errors.New("interface conversion error")
	}
	switch head.SubType {
	case MsgSDWANInfoReq:
		rsp := buildSDWANInfo(sess.user, sess.node)
		sess.write(MsgSDWAN, MsgSDWANInfoRsp, &rsp)
	default:
		return nil
	}
	return nil
}

// buildSDWANInfo returns the SDWAN config for the network that contains node.
// If the node is not in any enabled network, an empty SDWANInfo is returned so
// the client can finish its initialization normally.
func buildSDWANInfo(user, node string) SDWANInfo {
	rsp := SDWANInfo{Nodes: []*SDWANNode{}}
	if gStore == nil {
		return rsp
	}
	n := gStore.findNetworkByNode(user, node)
	if n == nil {
		return rsp
	}
	rsp.ID = n.ID
	rsp.Name = n.Name
	rsp.Gateway = n.Gateway
	rsp.Mode = n.Mode
	rsp.CentralNode = n.CentralNode
	rsp.ForceRelay = n.ForceRelay
	rsp.PunchPriority = n.PunchPriority
	rsp.Enable = n.Enable
	rsp.TunnelNum = n.TunnelNum
	rsp.Mtu = n.Mtu
	for _, m := range n.Members {
		if m.Enable == 0 {
			continue
		}
		rsp.Nodes = append(rsp.Nodes, &SDWANNode{
			Name:     m.Name,
			IP:       m.IP,
			Resource: m.Resource,
			Enable:   m.Enable,
		})
	}
	return rsp
}
