package types

type BasicBlockData struct {
	// Height contains the block height
	Height uint64 `json:"height" gorm:"index:idx_height"`
	// TipsetHash contains the tipset hash
	TipsetCid string `json:"tipset_cid" gorm:"index:idx_tipset_cid"`
	// Blocks Cid
	BlocksCid []string `json:"blocks_cid" gorm:"type:Array(String);index:idx_blocks_cid"`
}

type BlockMetadata struct {
	NodeInfo
}
