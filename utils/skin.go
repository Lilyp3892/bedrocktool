package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"

	"github.com/bedrock-tool/bedrocktool/locale"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

type Skin struct {
	*protocol.Skin
}

type SkinGeometry_Old struct {
	SkinGeometryDescription
	Bones []any `json:"bones"`
}

type Number int

func (n *Number) MarshalJSON() ([]byte, error) {
	return strconv.AppendInt(nil, int64(*n), 10), nil
}

func (n *Number) UnmarshalJSON(b []byte) error {
	var jn json.Number
	err := json.Unmarshal(b, &jn)
	if err != nil {
		return err
	}

	i64, err := jn.Int64()
	if err == nil {
		*n = Number(i64)
		return nil
	}

	f64, err := jn.Float64()
	if err == nil {
		*n = Number(f64)
		return nil
	}

	return err
}

type SkinGeometryDescription struct {
	Identifier          string    `json:"identifier,omitempty"`
	TextureWidth        Number    `json:"texture_width"`
	TextureHeight       Number    `json:"texture_height"`
	VisibleBoundsWidth  float64   `json:"visible_bounds_width"`
	VisibleBoundsHeight float64   `json:"visible_bounds_height"`
	VisibleBoundsOffset []float64 `json:"visible_bounds_offset,omitempty"`
}

type SkinGeometry struct {
	Description SkinGeometryDescription `json:"description"`
	Bones       []Bone                  `json:"bones"`
}

type SkinGeometryFile struct {
	FormatVersion string         `json:"format_version"`
	Geometry      []SkinGeometry `json:"minecraft:geometry"`
}

type geometry180 struct {
	Bones         []Bone `json:"bones"`
	TextureWidth  int    `json:"texturewidth"`
	TextureHeight int    `json:"textureheight"`
}

type geom180 struct {
	m  map[string]geometry180
	id string
}

func (n *geom180) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"format_version": "1.8.0",
	}
	for k, v := range n.m {
		m[k] = v
	}
	return json.Marshal(m)
}

func (n *geom180) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	err := json.Unmarshal(b, &m)
	if err != nil {
		return err
	}
	if n.m == nil {
		n.m = make(map[string]geometry180)
	}
	var geom geometry180
	err = json.Unmarshal(m[n.id], &geom)
	if err != nil {
		return err
	}
	n.m[n.id] = geom
	return nil
}

type Bone struct {
	Name     string         `json:"name"`
	Parent   string         `json:"parent"`
	Pivot    []float64      `json:"pivot"`
	Rotation []float64      `json:"rotation"`
	Locators map[string]any `json:"locators"`
	Cubes    []Cube         `json:"cubes"`
}

type Cube struct {
	Mirror   bool      `json:"mirror"`
	Inflate  float64   `json:"inflate"`
	Pivot    []float64 `json:"pivot"`
	Rotation []float64 `json:"rotation"`
	Origin   []float64 `json:"origin"`
	Size     []float64 `json:"size"`
	UV       any       `json:"uv"`
}

func (skin *Skin) Hash() uuid.UUID {
	h := append(skin.CapeData, append(skin.SkinData, skin.SkinGeometry...)...)
	return uuid.NewSHA1(uuid.NameSpaceURL, h)
}

func ParseSkinGeometry(skin *protocol.Skin) (*SkinGeometryFile, string, error) {
	var resourcePatch map[string]map[string]string
	if len(skin.SkinResourcePatch) > 0 {
		err := ParseJson(skin.SkinResourcePatch, &resourcePatch)
		if err != nil {
			return nil, "", err
		}
	}
	var identifier string
	if resourcePatch != nil {
		identifier = resourcePatch["geometry"]["default"]
	}

	var data *struct {
		FormatVersion string         `json:"format_version"`
		Geometry      []SkinGeometry `json:"minecraft:geometry"`
	}
	err := ParseJson(skin.SkinGeometry, &data)
	if err != nil {
		return nil, identifier, err
	}
	if data == nil {
		return nil, identifier, nil
	}

	if data.FormatVersion == "1.8.0" {
		var m geom180 = geom180{
			id: identifier,
		}
		err := ParseJson(skin.SkinGeometry, &m)
		if err != nil {
			return nil, "", err
		}
		geom := m.m[identifier]
		return &SkinGeometryFile{
			FormatVersion: data.FormatVersion,
			Geometry: []SkinGeometry{
				{
					Description: SkinGeometryDescription{
						Identifier:    identifier,
						TextureWidth:  Number(geom.TextureWidth),
						TextureHeight: Number(geom.TextureHeight),
					},
					Bones: geom.Bones,
				},
			},
		}, identifier, nil
	}

	return &SkinGeometryFile{
		FormatVersion: string(skin.GeometryDataEngineVersion),
		Geometry:      data.Geometry,
	}, identifier, nil
}

func (skin *Skin) getGeometry() (*SkinGeometry, string, error) {
	if !skin.HaveGeometry() {
		return nil, "", errors.New("no geometry")
	}
	geom, identifier, err := ParseSkinGeometry(skin.Skin)
	if err != nil {
		return nil, "", err
	}
	return &geom.Geometry[0], identifier, nil
}

// WriteCape writes the cape as a png at output_path
func (skin *Skin) WriteCapePng(output_path string) error {
	f, err := os.Create(output_path)
	if err != nil {
		return errors.New(locale.Loc("failed_write", locale.Strmap{"Part": "Cape", "Path": output_path, "Err": err}))
	}
	defer f.Close()
	cape_tex := image.NewRGBA(image.Rect(0, 0, int(skin.CapeImageWidth), int(skin.CapeImageHeight)))
	cape_tex.Pix = skin.CapeData

	if err := png.Encode(f, cape_tex); err != nil {
		return fmt.Errorf(locale.Loc("failed_write", locale.Strmap{"Part": "Cape", "Err": err}))
	}
	return nil
}

// WriteTexture writes the main texture for this skin to a file
func (skin *Skin) writeSkinTexturePng(output_path string) error {
	f, err := os.Create(output_path)
	if err != nil {
		return errors.New(locale.Loc("failed_write", locale.Strmap{"Part": "Meta", "Path": output_path, "Err": err}))
	}
	defer f.Close()
	skin_tex := image.NewRGBA(image.Rect(0, 0, int(skin.SkinImageWidth), int(skin.SkinImageHeight)))
	skin_tex.Pix = skin.SkinData

	if err := png.Encode(f, skin_tex); err != nil {
		return errors.New(locale.Loc("failed_write", locale.Strmap{"Part": "Texture", "Path": output_path, "Err": err}))
	}
	return nil
}

func (skin *Skin) writeMetadataJson(output_path string) error {
	f, err := os.Create(output_path)
	if err != nil {
		return errors.New(locale.Loc("failed_write", locale.Strmap{"Part": "Meta", "Path": output_path, "Err": err}))
	}
	defer f.Close()
	d, err := json.MarshalIndent(SkinMeta{
		skin.SkinID,
		skin.PlayFabID,
		skin.PremiumSkin,
		skin.PersonaSkin,
		skin.CapeID,
		skin.SkinColour,
		skin.ArmSize,
		skin.Trusted,
		skin.PersonaPieces,
	}, "", "    ")
	if err != nil {
		return err
	}
	f.Write(d)
	return nil
}

func (skin *Skin) HaveGeometry() bool {
	return len(skin.SkinGeometry) > 0
}

func (skin *Skin) HaveCape() bool {
	return len(skin.CapeData) > 0
}

func (skin *Skin) HaveAnimations() bool {
	return len(skin.Animations) > 0
}

func (skin *Skin) HaveTint() bool {
	return len(skin.PieceTintColours) > 0
}

func (skin *Skin) Complex() bool {
	return skin.HaveGeometry() || skin.HaveCape() || skin.HaveAnimations() || skin.HaveTint()
}
