package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	connectResult map[string]int
	resultMtx     sync.Mutex
)

type reportHandler struct {
}

func (h *reportHandler) handleMessage(ctx *msgContext) error {
	head := openP2PHeader{}
	err := binary.Read(bytes.NewReader(ctx.msg[:openP2PHeaderSize]), binary.LittleEndian, &head)
	if err != nil {
		return err
	}
	wsSess, ok := ctx.sess.(*wssSession)
	if !ok {
		gLog.Println(LvERROR, "interface conversion error")
		return errors.New("interface conversion error")
	}
	switch head.SubType {
	case MsgReportBasic:
		// gLog.Println(LvINFO, "MsgReportBasic")
		req := ReportBasic{}
		err = json.Unmarshal(ctx.msg[openP2PHeaderSize:], &req)
		if err != nil {
			gLog.Printf(LvERROR, "wrong ReportBasic:%s", err)
			return err
		}
		wsSess.os = req.OS
		wsSess.mac = req.Mac
		wsSess.lanIP = req.LanIP
		wsSess.hasIPv4 = req.HasIPv4
		wsSess.hasUPNPorNATPMP = req.HasUPNPorNATPMP
		wsSess.IPv6 = req.IPv6
		wsSess.version = req.Version
		wsSess.majorVer = parseMajorVer(req.Version)
		PushNotifyChan(wsSess.node)
		// client waits for MsgReportBasicRsp, otherwise it exits
		wsSess.write(MsgReport, MsgReportBasicRsp, nil)
		// persist enriched device info(os/mac/lanip/ipv6)
		gStore.upsertDevice(&StoreDevice{
			Name:       wsSess.node,
			User:       wsSess.user,
			OS:         wsSess.os,
			MAC:        wsSess.mac,
			LanIP:      wsSess.lanIP,
			IPv4:       wsSess.IPv4,
			IPv6:       wsSess.IPv6,
			NatType:    wsSess.natType,
			Bandwidth:  wsSess.shareBandWidth,
			Version:    wsSess.version,
			Activetime: time.Now().Format("2006-01-02 15:04:05"),
		})
	case MsgReportQuery:
		gLog.Println(LvINFO, "MsgReportQuery")
		wsSess.rspCh <- ctx.msg[openP2PHeaderSize:]
	case MsgReportApps:
		gLog.Println(LvINFO, "MsgReportApps")
		wsSess.rspCh <- ctx.msg[openP2PHeaderSize:]
	case MsgPushReportLog:
		gLog.Println(LvINFO, "MsgPushReportLog")
		wsSess.rspCh <- ctx.msg[openP2PHeaderSize:]
	case MsgReportConnect:
		req := ReportConnect{}
		err = json.Unmarshal(ctx.msg[openP2PHeaderSize:], &req)
		if err != nil {
			gLog.Printf(LvERROR, "wrong MsgReportConnect:%s", err)
			return err
		}
		resultMtx.Lock()
		connectResult[req.Error]++
		if req.Error == "" {
			gLog.Println(LvINFO, "MsgReportConnect OK:", wsSess.node, string(ctx.msg[openP2PHeaderSize:]))
			connectResult["totalP2PConnectOK"]++
		} else {
			gLog.Println(LvERROR, "MsgReportConnect Error:", wsSess.node, string(ctx.msg[openP2PHeaderSize:]))
			wsSess.failNodes.Store(nodeNameToID(req.PeerNode), time.Now())
		}
		resultMtx.Unlock()
	default:
		return nil
	}
	return nil
}

func init() {
	connectResult = make(map[string]int)
}
