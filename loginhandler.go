package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"
)

type loginHandler struct {
}

func (h *loginHandler) handleMessage(ctx *msgContext) error {
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
	switch head.MainType {
	case MsgLogin:
		// gLogger.Println(LvINFO, string(msg))
		rsp := LoginRsp{}
		err = json.Unmarshal(ctx.msg[openP2PHeaderSize:], &rsp)
		if err != nil {
			wsSess.close()
			gLog.Printf(LvERROR, "wrong login response:%s", err)
			return err
		}
		if rsp.Error != 0 {
			wsSess.close()
			gLog.Printf(LvERROR, "login error:%d", rsp.Error)
		} else {
			gLog.Printf(LvINFO, "%s login ok", wsSess.node)
		}
	case MsgHeartbeat:
		wsSess.activeTime = time.Now()
		// client expects an 8-byte little-endian server timestamp(ns) as body for clock sync
		tsBody := make([]byte, 8)
		binary.LittleEndian.PutUint64(tsBody, uint64(time.Now().UnixNano()))
		msg := append(encodeHeader(MsgHeartbeat, 0, uint32(len(tsBody))), tsBody...)
		wsSess.writeBuff(msg)
		// gLog.Printf(LvINFO, "%s heartbeat ok", wsSess.node)
	default:
		return nil
	}
	return nil
}
