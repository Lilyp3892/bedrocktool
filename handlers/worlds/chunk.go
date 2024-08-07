package worlds

import (
	"bytes"
	"errors"
	"time"

	"github.com/bedrock-tool/bedrocktool/locale"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (w *worldsHandler) processLevelChunk(pk *packet.LevelChunk, timeReceived time.Time) (err error) {
	if len(pk.RawPayload) == 0 {
		w.log.Info(locale.Loc("empty_chunk", nil))
		return
	}

	var subChunkCount int
	switch pk.SubChunkCount {
	case protocol.SubChunkRequestModeLimited, protocol.SubChunkRequestModeLimitless:
		subChunkCount = 0
	default:
		subChunkCount = int(pk.SubChunkCount)
	}

	w.worldStateLock.Lock()
	defer w.worldStateLock.Unlock()

	//os.WriteFile("chunk.bin", pk.RawPayload, 0777)

	if pk.CacheEnabled {
		return errors.New("cache is supposed to be handled in proxy")
	}

	ch, blockNBTs, err := chunk.NetworkDecode(
		w.serverState.blocks,
		pk.RawPayload, subChunkCount,
		w.serverState.useOldBiomes,
		w.serverState.useHashedRids,
		w.currentWorld.Range(),
	)
	if err != nil {
		return err
	}

	col := &world.Column{
		Chunk:         ch,
		BlockEntities: make(map[cube.Pos]world.Block),
	}

	for _, blockNBT := range blockNBTs {
		x := int(blockNBT["x"].(int32))
		y := int(blockNBT["y"].(int32))
		z := int(blockNBT["z"].(int32))
		col.BlockEntities[cube.Pos{x, y, z}] = world.UnknownBlock{
			BlockState: world.BlockState{
				Name:       blockNBT["id"].(string),
				Properties: blockNBT,
			},
		}
	}

	pos := world.ChunkPos(pk.Position)
	if !w.scripting.OnChunkAdd(pos, timeReceived) {
		w.currentWorld.IgnoredChunks[pos] = true
		return
	}
	w.currentWorld.IgnoredChunks[pos] = false

	err = w.currentWorld.StoreChunk(pos, col)
	if err != nil {
		w.log.Error(err)
	}

	max := w.currentWorld.Dimension().Range().Height() / 16
	switch pk.SubChunkCount {
	case protocol.SubChunkRequestModeLimited:
		max = int(pk.HighestSubChunk)
		fallthrough
	case protocol.SubChunkRequestModeLimitless:
		var offsetTable []protocol.SubChunkOffset
		r := w.currentWorld.Dimension().Range()
		for y := int8(r.Min() / 16); y < int8(r.Max()/16)+1; y++ {
			offsetTable = append(offsetTable, protocol.SubChunkOffset{0, y, 0})
		}

		dimId, _ := world.DimensionID(w.currentWorld.Dimension())
		_ = w.session.Server.WritePacket(&packet.SubChunkRequest{
			Dimension: int32(dimId),
			Position: protocol.SubChunkPos{
				pk.Position.X(), 0, pk.Position.Z(),
			},
			Offsets: offsetTable[:min(max+1, len(offsetTable))],
		})
	default:
	}

	w.session.SendPopup(locale.Locm("popup_chunk_count", locale.Strmap{
		"Chunks":   len(w.currentWorld.StoredChunks),
		"Entities": w.currentWorld.EntityCount(),
		"Name":     w.currentWorld.Name,
	}, len(w.currentWorld.StoredChunks)))

	return nil
}

func (w *worldsHandler) processSubChunk(pk *packet.SubChunk) error {
	w.worldStateLock.Lock()
	defer w.worldStateLock.Unlock()

	var columns = make(map[world.ChunkPos]*world.Column)

	for _, ent := range pk.SubChunkEntries {
		if ent.Result != protocol.SubChunkResultSuccess {
			continue
		}
		var (
			absX = pk.Position[0] + int32(ent.Offset[0])
			absZ = pk.Position[2] + int32(ent.Offset[2])
			pos  = world.ChunkPos{absX, absZ}
		)

		if w.currentWorld.IgnoredChunks[pos] {
			continue
		}

		if _, ok := columns[pos]; ok {
			continue
		}
		col, ok, err := w.currentWorld.LoadChunk(pos)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("bug check: subchunk received before chunk")
		}
		columns[pos] = col
	}

	for _, ent := range pk.SubChunkEntries {
		var (
			absX = pk.Position[0] + int32(ent.Offset[0])
			absY = pk.Position[1] + int32(ent.Offset[1])
			absZ = pk.Position[2] + int32(ent.Offset[2])
			pos  = world.ChunkPos{absX, absZ}
		)

		col, ok := columns[pos]
		if !ok {
			continue
		}

		switch ent.Result {
		case protocol.SubChunkResultSuccessAllAir:
		case protocol.SubChunkResultSuccess:
			buf := bytes.NewBuffer(ent.RawPayload)
			index := uint8(absY)
			sub, err := chunk.DecodeSubChunk(
				buf,
				w.serverState.blocks,
				w.currentWorld.Dimension().Range(),
				&index,
				chunk.NetworkEncoding,
				w.serverState.useHashedRids,
			)
			if err != nil {
				return err
			}
			col.Chunk.Sub()[index] = sub

			if buf.Len() > 0 {
				dec := nbt.NewDecoderWithEncoding(buf, nbt.NetworkLittleEndian)
				for buf.Len() > 0 {
					blockNBT := make(map[string]any, 0)
					if err := dec.Decode(&blockNBT); err != nil {
						return err
					}
					col.BlockEntities[cube.Pos{
						int(blockNBT["x"].(int32)),
						int(blockNBT["y"].(int32)),
						int(blockNBT["z"].(int32)),
					}] = world.UnknownBlock{
						BlockState: world.BlockState{
							Name:       blockNBT["id"].(string),
							Properties: blockNBT,
						},
					}
				}
			}
		}
	}

	for pos, col := range columns {
		w.currentWorld.StoreChunk(pos, col)
	}

	w.mapUI.SchedRedraw()
	return nil
}
