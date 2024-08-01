package worldstate

import (
	"image"
	"image/draw"
	"maps"

	"github.com/bedrock-tool/bedrocktool/handlers/worlds/entity"
	"github.com/bedrock-tool/bedrocktool/utils"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/thomaso-mirodin/intmath/i32"
)

type worldStateMem struct {
	maps        map[int64]*Map
	chunks      map[world.ChunkPos]*chunk.Chunk
	blockNBTs   map[world.ChunkPos]map[cube.Pos]world.UnknownBlock
	entities    map[entity.RuntimeID]*entity.Entity
	entityLinks map[entity.UniqueID]map[entity.UniqueID]struct{}

	uniqueIDsToRuntimeIDs map[entity.UniqueID]entity.RuntimeID
}

func (w *worldStateMem) StoreChunk(pos world.ChunkPos, ch *chunk.Chunk, blockNBT map[cube.Pos]world.UnknownBlock) {
	w.chunks[pos] = ch
	if blockNBT != nil {
		w.blockNBTs[pos] = blockNBT
	}
}

func (w *worldStateMem) StoreMap(m *packet.ClientBoundMapItemData) {
	return // not finished yet
	m1, ok := w.maps[m.MapID]
	if !ok {
		m1 = &Map{
			MapID:     m.MapID,
			Height:    128,
			Width:     128,
			Scale:     1,
			Dimension: 0,
			ZCenter:   m.Origin.Z(),
			XCenter:   m.Origin.X(),
		}
		w.maps[m.MapID] = m1
	}
	draw.Draw(&image.RGBA{
		Pix:    m1.Colors[:],
		Rect:   image.Rect(0, 0, int(m.Width), int(m.Height)),
		Stride: int(m.Width) * 4,
	}, image.Rect(
		int(m.XOffset), int(m.YOffset),
		int(m.Width), int(m.Height),
	), utils.RGBA2Img(
		m.Pixels,
		image.Rect(
			0, 0,
			int(m.Width), int(m.Height),
		),
	), image.Point{}, draw.Over)
}

func (w *worldStateMem) cullChunks() {
	for key, ch := range w.chunks {
		var empty = true
		for _, sub := range ch.Sub() {
			if !sub.Empty() {
				empty = false
				break
			}
		}
		if empty {
			delete(w.chunks, key)
		}
	}
}

func (w *worldStateMem) ApplyTo(w2 worldStateInterface, around cube.Pos, radius int32, cf func(world.ChunkPos, *chunk.Chunk)) {
	w.cullChunks()
	for cp, c := range w.chunks {
		dist := i32.Sqrt(i32.Pow(cp.X()-int32(around.X()/16), 2) + i32.Pow(cp.Z()-int32(around.Z()/16), 2))
		blockNBT := w.blockNBTs[cp]
		if dist <= radius || radius < 0 {
			w2.StoreChunk(cp, c, blockNBT)
			cf(cp, c)
		} else {
			cf(cp, nil)
		}
	}

	for k, es := range w.entities {
		x := int(es.Position[0])
		z := int(es.Position[2])
		dist := i32.Sqrt(i32.Pow(int32(x-around.X()), 2) + i32.Pow(int32(z-around.Z()), 2))
		e2 := w2.GetEntity(k)
		if e2 != nil || dist < radius*16 || radius < 0 {
			w2.StoreEntity(k, es)
		}
	}
}

func cubePosInChunk(pos cube.Pos) (p world.ChunkPos, sp int16) {
	p[0] = int32(pos.X() >> 4)
	sp = int16(pos.Y() >> 4)
	p[1] = int32(pos.Z() >> 4)
	return
}

func (w *worldStateMem) SetBlockNBT(pos cube.Pos, m map[string]any, merge bool) {
	cp, _ := cubePosInChunk(pos)
	chunkNBTs, ok := w.blockNBTs[cp]
	if !ok {
		chunkNBTs = make(map[cube.Pos]world.UnknownBlock)
		w.blockNBTs[cp] = chunkNBTs
	}
	b, ok := chunkNBTs[pos]
	if !ok {
		b = world.UnknownBlock{
			BlockState: world.BlockState{
				Name:       m["id"].(string),
				Properties: m,
			},
		}
	}

	if merge {
		maps.Copy(b.Properties, m)
	} else {
		b.Properties = m
	}
	chunkNBTs[pos] = b
}

func (w *worldStateMem) StoreEntity(id entity.RuntimeID, es *entity.Entity) {
	w.entities[id] = es
	w.uniqueIDsToRuntimeIDs[es.UniqueID] = es.RuntimeID
}

func (w *worldStateMem) GetEntity(id entity.RuntimeID) *entity.Entity {
	return w.entities[id]
}

func (w *worldStateMem) AddEntityLink(el protocol.EntityLink) {
	switch el.Type {
	case protocol.EntityLinkPassenger:
		fallthrough
	case protocol.EntityLinkRider:
		if _, ok := w.entityLinks[el.RiddenEntityUniqueID]; !ok {
			w.entityLinks[el.RiddenEntityUniqueID] = make(map[int64]struct{})
		}
		w.entityLinks[el.RiddenEntityUniqueID][el.RiderEntityUniqueID] = struct{}{}
	case protocol.EntityLinkRemove:
		delete(w.entityLinks[el.RiddenEntityUniqueID], el.RiderEntityUniqueID)
	}
}
