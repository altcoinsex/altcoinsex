// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockdag

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/daglabs/btcd/dagconfig/daghash"
	"github.com/daglabs/btcd/database"
	"github.com/daglabs/btcd/wire"
)

// TestErrNotInDAG ensures the functions related to errNotInDAG work
// as expected.
func TestErrNotInDAG(t *testing.T) {
	errStr := "no block at height 1 exists"
	err := error(errNotInDAG(errStr))

	// Ensure the stringized output for the error is as expected.
	if err.Error() != errStr {
		t.Fatalf("errNotInDAG retuned unexpected error string - "+
			"got %q, want %q", err.Error(), errStr)
	}

	// Ensure error is detected as the correct type.
	if !isNotInDAGErr(err) {
		t.Fatalf("isNotInDAGErr did not detect as expected type")
	}
	err = errors.New("something else")
	if isNotInDAGErr(err) {
		t.Fatalf("isNotInDAGErr detected incorrect type")
	}
}

// TestStxoSerialization ensures serializing and deserializing spent transaction
// output entries works as expected.
func TestStxoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stxo       spentTxOut
		serialized []byte
	}{
		// From block 170 in main blockchain.
		{
			name: "Spends last output of coinbase",
			stxo: spentTxOut{
				amount:     5000000000,
				pkScript:   hexToBytes("410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"),
				isCoinBase: true,
				height:     9,
			},
			serialized: hexToBytes("1300320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c"),
		},
		// Adapted from block 100025 in main blockchain.
		{
			name: "Spends last output of non coinbase",
			stxo: spentTxOut{
				amount:     13761000000,
				pkScript:   hexToBytes("76a914b2fb57eadf61e106a100a7445a8c3f67898841ec88ac"),
				isCoinBase: false,
				height:     100024,
			},
			serialized: hexToBytes("8b99700086c64700b2fb57eadf61e106a100a7445a8c3f67898841ec"),
		},
		// Adapted from block 100025 in main blockchain.
		{
			name: "Does not spend last output, legacy format",
			stxo: spentTxOut{
				amount:   34405000000,
				pkScript: hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688ac"),
			},
			serialized: hexToBytes("0091f20f006edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		},
	}

	for _, test := range tests {
		// Ensure the function to calculate the serialized size without
		// actually serializing it is calculated properly.
		gotSize := spentTxOutSerializeSize(&test.stxo)
		if gotSize != len(test.serialized) {
			t.Errorf("spentTxOutSerializeSize (%s): did not get "+
				"expected size - got %d, want %d", test.name,
				gotSize, len(test.serialized))
			continue
		}

		// Ensure the stxo serializes to the expected value.
		gotSerialized := make([]byte, gotSize)
		gotBytesWritten := putSpentTxOut(gotSerialized, &test.stxo)
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("putSpentTxOut (%s): did not get expected "+
				"bytes - got %x, want %x", test.name,
				gotSerialized, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("putSpentTxOut (%s): did not get expected "+
				"number of bytes written - got %d, want %d",
				test.name, gotBytesWritten,
				len(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// stxo.
		var gotStxo spentTxOut
		gotBytesRead, err := decodeSpentTxOut(test.serialized, &gotStxo)
		if err != nil {
			t.Errorf("decodeSpentTxOut (%s): unexpected error: %v",
				test.name, err)
			continue
		}
		if !reflect.DeepEqual(gotStxo, test.stxo) {
			t.Errorf("decodeSpentTxOut (%s) mismatched entries - "+
				"got %v, want %v", test.name, gotStxo, test.stxo)
			continue
		}
		if gotBytesRead != len(test.serialized) {
			t.Errorf("decodeSpentTxOut (%s): did not get expected "+
				"number of bytes read - got %d, want %d",
				test.name, gotBytesRead, len(test.serialized))
			continue
		}
	}
}

// TestStxoDecodeErrors performs negative tests against decoding spent
// transaction outputs to ensure error paths work as expected.
func TestStxoDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stxo       spentTxOut
		serialized []byte
		bytesRead  int // Expected number of bytes read.
		errType    error
	}{
		{
			name:       "nothing serialized",
			stxo:       spentTxOut{},
			serialized: hexToBytes(""),
			errType:    errDeserialize(""),
			bytesRead:  0,
		},
		{
			name:       "no data after header code w/o reserved",
			stxo:       spentTxOut{},
			serialized: hexToBytes("00"),
			errType:    errDeserialize(""),
			bytesRead:  1,
		},
		{
			name:       "no data after header code with reserved",
			stxo:       spentTxOut{},
			serialized: hexToBytes("13"),
			errType:    errDeserialize(""),
			bytesRead:  1,
		},
		{
			name:       "no data after reserved",
			stxo:       spentTxOut{},
			serialized: hexToBytes("1300"),
			errType:    errDeserialize(""),
			bytesRead:  2,
		},
		{
			name:       "incomplete compressed txout",
			stxo:       spentTxOut{},
			serialized: hexToBytes("1332"),
			errType:    errDeserialize(""),
			bytesRead:  2,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		gotBytesRead, err := decodeSpentTxOut(test.serialized,
			&test.stxo)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("decodeSpentTxOut (%s): expected error type "+
				"does not match - got %T, want %T", test.name,
				err, test.errType)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("decodeSpentTxOut (%s): unexpected number of "+
				"bytes read - got %d, want %d", test.name,
				gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestSpendJournalSerialization ensures serializing and deserializing spend
// journal entries works as expected.
func TestSpendJournalSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entry      []spentTxOut
		blockTxns  []*wire.MsgTx
		serialized []byte
	}{
		// From block 2 in main blockchain.
		{
			name:       "No spends",
			entry:      nil,
			blockTxns:  nil,
			serialized: nil,
		},
		// From block 170 in main blockchain.
		{
			name: "One tx with one input spends last output of coinbase",
			entry: []spentTxOut{{
				amount:     5000000000,
				pkScript:   hexToBytes("410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"),
				isCoinBase: true,
				height:     9,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        math.MaxUint64,
				}},
				TxOut: []*wire.TxOut{{
					Value:    1000000000,
					PkScript: hexToBytes("4104ae1a62fe09c5f51b13905f07f06b99a2f7159b2225f374cd378d71302fa28414e7aab37397f554a7df5f142c21c1b7303b8a0626f1baded5c72a704f7e6cd84cac"),
				}, {
					Value:    4000000000,
					PkScript: hexToBytes("410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"),
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes("1300320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c"),
		},
		// Adapted from block 100025 in main blockchain.
		{
			name: "Two txns when one spends last output, one doesn't",
			entry: []spentTxOut{{
				amount:     34405000000,
				pkScript:   hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688ac"),
				isCoinBase: false,
				height:     100024,
			}, {
				amount:     13761000000,
				pkScript:   hexToBytes("76a914b2fb57eadf61e106a100a7445a8c3f67898841ec88ac"),
				isCoinBase: false,
				height:     100024,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"),
						Index: 1,
					},
					SignatureScript: hexToBytes("493046022100c167eead9840da4a033c9a56470d7794a9bb1605b377ebe5688499b39f94be59022100fb6345cab4324f9ea0b9ee9169337534834638d818129778370f7d378ee4a325014104d962cac5390f12ddb7539507065d0def320d68c040f2e73337c3a1aaaab7195cb5c4d02e0959624d534f3c10c3cf3d73ca5065ebd62ae986b04c6d090d32627c"),
					Sequence:        math.MaxUint64,
				}},
				TxOut: []*wire.TxOut{{
					Value:    5000000,
					PkScript: hexToBytes("76a914f419b8db4ba65f3b6fcc233acb762ca6f51c23d488ac"),
				}, {
					Value:    34400000000,
					PkScript: hexToBytes("76a914cadf4fc336ab3c6a4610b75f31ba0676b7f663d288ac"),
				}},
				LockTime: 0,
			}, {
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("92fbe1d4be82f765dfabc9559d4620864b05cc897c4db0e29adac92d294e52b7"),
						Index: 0,
					},
					SignatureScript: hexToBytes("483045022100e256743154c097465cf13e89955e1c9ff2e55c46051b627751dee0144183157e02201d8d4f02cde8496aae66768f94d35ce54465bd4ae8836004992d3216a93a13f00141049d23ce8686fe9b802a7a938e8952174d35dd2c2089d4112001ed8089023ab4f93a3c9fcd5bfeaa9727858bf640dc1b1c05ec3b434bb59837f8640e8810e87742"),
					Sequence:        math.MaxUint64,
				}},
				TxOut: []*wire.TxOut{{
					Value:    5000000,
					PkScript: hexToBytes("76a914a983ad7c92c38fc0e2025212e9f972204c6e687088ac"),
				}, {
					Value:    13756000000,
					PkScript: hexToBytes("76a914a6ebd69952ab486a7a300bfffdcb395dc7d47c2388ac"),
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes("8b99700086c64700b2fb57eadf61e106a100a7445a8c3f67898841ec8b99700091f20f006edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		},
	}

	for i, test := range tests {
		// Ensure the journal entry serializes to the expected value.
		gotBytes := serializeSpendJournalEntry(test.entry)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeSpendJournalEntry #%d (%s): "+
				"mismatched bytes - got %x, want %x", i,
				test.name, gotBytes, test.serialized)
			continue
		}

		// Deserialize to a spend journal entry.
		gotEntry, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns)
		if err != nil {
			t.Errorf("deserializeSpendJournalEntry #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}

		// Ensure that the deserialized spend journal entry has the
		// correct properties.
		if !reflect.DeepEqual(gotEntry, test.entry) {
			t.Errorf("deserializeSpendJournalEntry #%d (%s) "+
				"mismatched entries - got %v, want %v",
				i, test.name, gotEntry, test.entry)
			continue
		}
	}
}

// TestSpendJournalErrors performs negative tests against deserializing spend
// journal entries to ensure error paths work as expected.
func TestSpendJournalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		blockTxns  []*wire.MsgTx
		serialized []byte
		errType    error
	}{
		// Adapted from block 170 in main blockchain.
		{
			name: "Force assertion due to missing stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        math.MaxUint64,
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes(""),
			errType:    AssertError(""),
		},
		{
			name: "Force deserialization error in stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        math.MaxUint64,
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes("1301320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a"),
			errType:    errDeserialize(""),
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the returned
		// slice is nil.
		stxos, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeSpendJournalEntry (%s): expected "+
				"error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if stxos != nil {
			t.Errorf("deserializeSpendJournalEntry (%s): returned "+
				"slice of spent transaction outputs is not nil",
				test.name)
			continue
		}
	}
}

// TestUtxoSerialization ensures serializing and deserializing unspent
// trasaction output entries works as expected.
func TestUtxoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entry      *UTXOEntry
		serialized []byte
	}{
		// From tx in main blockchain:
		// b7c3332bc138e2c9429818f5fed500bcc1746544218772389054dc8047d7cd3f:0
		{
			name: "height 1, coinbase",
			entry: &UTXOEntry{
				amount:      5000000000,
				pkScript:    hexToBytes("410496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52da7589379515d4e0a604f8141781e62294721166bf621e73a82cbf2342c858eeac"),
				blockHeight: 1,
				packedFlags: tfCoinBase,
			},
			serialized: hexToBytes("03320496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52"),
		},
		// From tx in main blockchain:
		// 8131ffb0a2c945ecaf9b9063e59558784f9c3a74741ce6ae2a18d0571dac15bb:1
		{
			name: "height 100001, not coinbase",
			entry: &UTXOEntry{
				amount:      1000000,
				pkScript:    hexToBytes("76a914ee8bd501094a7d5ca318da2506de35e1cb025ddc88ac"),
				blockHeight: 100001,
				packedFlags: 0,
			},
			serialized: hexToBytes("8b99420700ee8bd501094a7d5ca318da2506de35e1cb025ddc"),
		},
	}

	for i, test := range tests {
		// Ensure the utxo entry serializes to the expected value.
		gotBytes, err := serializeUTXOEntry(test.entry)
		if err != nil {
			t.Errorf("serializeUTXOEntry #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeUTXOEntry #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Deserialize to a utxo entry.
		utxoEntry, err := deserializeUTXOEntry(test.serialized)
		if err != nil {
			t.Errorf("deserializeUTXOEntry #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}

		// Ensure the deserialized entry has the same properties as the
		// ones in the test entry.
		if utxoEntry.Amount() != test.entry.Amount() {
			t.Errorf("deserializeUTXOEntry #%d (%s) mismatched "+
				"amounts: got %d, want %d", i, test.name,
				utxoEntry.Amount(), test.entry.Amount())
			continue
		}

		if !bytes.Equal(utxoEntry.PkScript(), test.entry.PkScript()) {
			t.Errorf("deserializeUTXOEntry #%d (%s) mismatched "+
				"scripts: got %x, want %x", i, test.name,
				utxoEntry.PkScript(), test.entry.PkScript())
			continue
		}
		if utxoEntry.BlockHeight() != test.entry.BlockHeight() {
			t.Errorf("deserializeUTXOEntry #%d (%s) mismatched "+
				"block height: got %d, want %d", i, test.name,
				utxoEntry.BlockHeight(), test.entry.BlockHeight())
			continue
		}
		if utxoEntry.IsCoinBase() != test.entry.IsCoinBase() {
			t.Errorf("deserializeUTXOEntry #%d (%s) mismatched "+
				"coinbase flag: got %v, want %v", i, test.name,
				utxoEntry.IsCoinBase(), test.entry.IsCoinBase())
			continue
		}
	}
}

// TestUtxoEntryDeserializeErrors performs negative tests against deserializing
// unspent transaction outputs to ensure error paths work as expected.
func TestUtxoEntryDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errType    error
	}{
		{
			name:       "no data after header code",
			serialized: hexToBytes("02"),
			errType:    errDeserialize(""),
		},
		{
			name:       "incomplete compressed txout",
			serialized: hexToBytes("0232"),
			errType:    errDeserialize(""),
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the returned
		// entry is nil.
		entry, err := deserializeUTXOEntry(test.serialized)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeUTXOEntry (%s): expected error "+
				"type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if entry != nil {
			t.Errorf("deserializeUTXOEntry (%s): returned entry "+
				"is not nil", test.name)
			continue
		}
	}
}

// TestDAGTipHashesSerialization ensures serializing and deserializing the
// DAG tip hashes works as expected.
func TestDAGTipHashesSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tipHashes  []daghash.Hash
		serialized []byte
	}{
		{
			name:       "genesis",
			tipHashes:  []daghash.Hash{*newHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")},
			serialized: []byte("[[111,226,140,10,182,241,179,114,193,166,162,70,174,99,247,79,147,30,131,101,225,90,8,156,104,214,25,0,0,0,0,0]]"),
		},
		{
			name:       "block 1",
			tipHashes:  []daghash.Hash{*newHashFromStr("00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048")},
			serialized: []byte("[[72,96,235,24,191,27,22,32,227,126,148,144,252,138,66,117,20,65,111,215,81,89,171,134,104,142,154,131,0,0,0,0]]"),
		},
	}

	for i, test := range tests {
		gotBytes, err := serializeDAGTipHashes(test.tipHashes)
		if err != nil {
			t.Errorf("serializeDAGTipHashes #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}

		// Ensure the tipHashes serializes to the expected value.
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeDAGTipHashes #%d (%s): mismatched "+
				"bytes - got %s, want %s", i, test.name,
				string(gotBytes), string(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// tipHashes.
		tipHashes, err := deserializeDAGTipHashes(test.serialized)
		if err != nil {
			t.Errorf("deserializeDAGTipHashes #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(tipHashes, test.tipHashes) {
			t.Errorf("deserializeDAGTipHashes #%d (%s) "+
				"mismatched tipHashes - got %v, want %v", i,
				test.name, tipHashes, test.tipHashes)
			continue
		}
	}
}

// TestDAGTipHashesDeserializeErrors performs negative tests against
// deserializing the DAG tip hashes to ensure error paths work as expected.
func TestDAGTipHashesDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errType    error
	}{
		{
			name:       "nothing serialized",
			serialized: hexToBytes(""),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
		{
			name:       "corrupted data",
			serialized: []byte("[[111,226,140,10,182,241,179,114,193,166,162,70,174,99,247,7"),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
	}

	for _, test := range tests {
		// Ensure the expected error type and code is returned.
		_, err := deserializeDAGTipHashes(test.serialized)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeDAGTipHashes (%s): expected "+
				"error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if derr, ok := err.(database.Error); ok {
			tderr := test.errType.(database.Error)
			if derr.ErrorCode != tderr.ErrorCode {
				t.Errorf("deserializeDAGTipHashes (%s): "+
					"wrong error code got: %v, want: %v",
					test.name, derr.ErrorCode,
					tderr.ErrorCode)
				continue
			}
		}
	}
}