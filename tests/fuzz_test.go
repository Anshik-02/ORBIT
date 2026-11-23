package tests

import (
	"encoding/json"
	"testing"

	"orbit/internal/api"
	"orbit/internal/sharding"
	"orbit/internal/storage"
	"orbit/internal/wal"
)

// FuzzWALRecordDecode tests WAL record decoding against arbitrary corrupt or truncated byte streams.
func FuzzWALRecordDecode(f *testing.F) {
	// Seed corpus with valid WAL records
	f.Add([]byte(`{"sequence":1,"raft_index":1,"raft_term":1,"type":"SET","key":"foo","value":"value"}`))
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		var rec wal.Record
		_ = json.Unmarshal(data, &rec)
		// Decode should never panic regardless of arbitrary payload
	})
}

// FuzzBloomFilterDecode tests Bloom filter deserialization against arbitrary corrupted bytes.
func FuzzBloomFilterDecode(f *testing.F) {
	bf := storage.NewBloomFilter(100, 0.01)
	bf.Add("alpha")
	bf.Add("beta")
	encoded := bf.Encode()

	f.Add(encoded)
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})

	f.Fuzz(func(t *testing.T, data []byte) {
		// DecodeBloomFilter must gracefully return an error and never panic on random inputs
		_, _ = storage.DecodeBloomFilter(data)
	})
}

// FuzzJSONPayloadDecode tests API unmarshaling against arbitrary or malformed JSON payloads.
func FuzzJSONPayloadDecode(f *testing.F) {
	f.Add([]byte(`{"value":"hello world"}`))
	f.Add([]byte(`{"ops":[{"type":"PUT","key":"k1","value":"v1"}]}`))
	f.Add([]byte(`{"invalid":`))
	f.Add([]byte(`{"cluster_id":"c1","epoch":1}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var putPayload struct {
			Value string `json:"value"`
		}
		_ = json.Unmarshal(data, &putPayload)

		var batchPayload api.BatchWriteRequest
		_ = json.Unmarshal(data, &batchPayload)

		var metaPayload sharding.ClusterMetadata
		_ = json.Unmarshal(data, &metaPayload)
	})
}
