package utils

import (
	"bytes"
	"net"
	"reflect"

	"github.com/fatih/color"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

var Pool = packet.NewPool()

var muted_packets = []string{
	"packet.UpdateBlock",
	"packet.MoveActorAbsolute",
	"packet.SetActorMotion",
	"packet.SetTime",
	"packet.RemoveActor",
	"packet.AddActor",
	"packet.UpdateAttributes",
	"packet.Interact",
	"packet.LevelEvent",
	"packet.SetActorData",
	"packet.MoveActorDelta",
	"packet.MovePlayer",
	"packet.BlockActorData",
	"packet.PlayerAuthInput",
	"packet.LevelChunk",
	"packet.LevelSoundEvent",
	"packet.ActorEvent",
	"packet.NetworkChunkPublisherUpdate",
	"packet.UpdateSubChunkBlocks",
	"packet.SubChunk",
	"packet.SubChunkRequest",
	"packet.Animate",
	"packet.NetworkStackLatency",
	"packet.InventoryTransaction",
}

func PacketLogger(header packet.Header, payload []byte, src, dst net.Addr) {
	var pk packet.Packet
	if pkFunc, ok := Pool[header.PacketID]; ok {
		pk = pkFunc()
	} else {
		pk = &packet.Unknown{PacketID: header.PacketID}
	}
	pk.Unmarshal(protocol.NewReader(bytes.NewBuffer(payload), 0))

	pk_name := reflect.TypeOf(pk).String()[1:]
	if slices.Contains(muted_packets, pk_name) {
		return
	}

	switch pk := pk.(type) {
	case *packet.Disconnect:
		logrus.Infof("Disconnect: %s", pk.Message)
	}

	dir := color.GreenString("S") + "->" + color.CyanString("C")
	src_addr, _, _ := net.SplitHostPort(src.String())
	if IPPrivate(net.ParseIP(src_addr)) {
		dir = color.CyanString("C") + "->" + color.GreenString("S")
	}

	logrus.Debugf("%s 0x%02x, %s", dir, pk.ID(), pk_name)
}
