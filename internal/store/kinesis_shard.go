package store

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
)

const (
	// MaxLabKinesisShardCount is the lab soft cap for CreateStream ShardCount.
	MaxLabKinesisShardCount = 4
	// kinesisMaxHashKeyDec is 2^128-1, the EndingHashKey for a single-shard stream.
	kinesisMaxHashKeyDec = "340282366920938463463374607431768211455"
)

// LabKinesisShardID returns the lab shard id for index (shardId-000000000000 .. shardId-000000000003).
func LabKinesisShardID(index int) string {
	if index < 0 {
		index = 0
	}
	return fmt.Sprintf("shardId-%012d", index)
}

// HashPartitionKeyToShard maps a partition key to a shard index using MD5 (AWS-shaped).
// Same key always maps to the same shard for a given shardCount.
func HashPartitionKeyToShard(partitionKey string, shardCount int) int {
	if shardCount <= 1 {
		return 0
	}
	sum := md5.Sum([]byte(partitionKey))
	n := binary.BigEndian.Uint64(sum[:8])
	return int(n % uint64(shardCount))
}

// LabKinesisShardIDs returns shard ids for a stream with shardCount shards (clamped to lab max).
func LabKinesisShardIDs(shardCount int) []string {
	if shardCount <= 0 {
		shardCount = 1
	}
	if shardCount > MaxLabKinesisShardCount {
		shardCount = MaxLabKinesisShardCount
	}
	out := make([]string, shardCount)
	for i := 0; i < shardCount; i++ {
		out[i] = LabKinesisShardID(i)
	}
	return out
}

// LabKinesisHashKeyRange returns StartingHashKey / EndingHashKey for shard index in a stream.
func LabKinesisHashKeyRange(shardIndex, shardCount int) (starting, ending string) {
	if shardCount <= 1 {
		return "0", kinesisMaxHashKeyDec
	}
	if shardIndex < 0 {
		shardIndex = 0
	}
	if shardIndex >= shardCount {
		shardIndex = shardCount - 1
	}
	max := new(big.Int)
	max.SetString(kinesisMaxHashKeyDec, 10)
	// range size = (max+1) / shardCount; last shard ends at max
	span := new(big.Int).Add(max, big.NewInt(1))
	shardCountBig := big.NewInt(int64(shardCount))
	size := new(big.Int).Div(span, shardCountBig)
	start := new(big.Int).Mul(size, big.NewInt(int64(shardIndex)))
	var end *big.Int
	if shardIndex == shardCount-1 {
		end = max
	} else {
		end = new(big.Int).Sub(new(big.Int).Mul(size, big.NewInt(int64(shardIndex+1))), big.NewInt(1))
	}
	return start.String(), end.String()
}

func validLabKinesisShardID(shardID string, shardCount int) bool {
	shardID = strings.TrimSpace(shardID)
	if shardID == "" {
		return true // callers treat empty as shard 0
	}
	for _, id := range LabKinesisShardIDs(shardCount) {
		if id == shardID {
			return true
		}
	}
	return false
}

func normalizeLabKinesisShardID(shardID string) string {
	shardID = strings.TrimSpace(shardID)
	if shardID == "" {
		return LabKinesisShardID(0)
	}
	return shardID
}
