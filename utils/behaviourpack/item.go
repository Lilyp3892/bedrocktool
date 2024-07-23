package behaviourpack

import "github.com/sandertv/gophertunnel/minecraft/protocol"

type itemDescription struct {
	Category       string `json:"category"`
	Identifier     string `json:"identifier"`
	IsExperimental bool   `json:"is_experimental"`
}

type minecraftItem struct {
	Description itemDescription `json:"description"`
	Components  map[string]any  `json:"components,omitempty"`
}

type itemBehaviour struct {
	FormatVersion string        `json:"format_version"`
	MinecraftItem minecraftItem `json:"minecraft:item"`
}

func (bp *Pack) AddItem(item protocol.ItemEntry) {
	ns, _ := ns_name_split(item.Name)
	if ns == "minecraft" {
		return
	}

	bp.items[item.Name] = &itemBehaviour{
		FormatVersion: bp.formatVersion,
		MinecraftItem: minecraftItem{
			Description: itemDescription{
				Identifier:     item.Name,
				IsExperimental: true,
			},
			Components: make(map[string]any),
		},
	}
}

func (bp *Pack) ApplyComponentEntries(entries []protocol.ItemComponentEntry) {
	for _, ice := range entries {
		item, ok := bp.items[ice.Name]
		if !ok {
			continue
		}
		if components, ok := ice.Data["components"].(map[string]any); ok {
			if _, ok := components["minecraft:icon"]; !ok {
				if item_properties, ok := components["item_properties"].(map[string]any); ok {
					components["minecraft:icon"] = item_properties["minecraft:icon"]
				}
			}

			if icon, ok := components["minecraft:icon"].(map[string]any); ok {
				if textures, ok := icon["textures"].(map[string]any); ok {
					icon["textures"] = textures["default"]
				}
			}

			item.MinecraftItem.Components = components
		}
	}
}
