package blockchain

import (
	"bytes"
	"crypto/rand"
	"os"
	"testing"

	"github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA/auxpow"
	"github.com/elastos/Elastos.ELA/config"
	"github.com/elastos/Elastos.ELA/core"
	"github.com/elastos/Elastos.ELA/log"
	"github.com/stretchr/testify/assert"
)

var txPool TxPool

func TestTxPoolInit(t *testing.T) {
	log.Init(
		config.Parameters.PrintLevel,
		config.Parameters.MaxPerLogSize,
		config.Parameters.MaxLogsSize,
	)
	foundation, err := common.Uint168FromAddress("8VYXVxKKSAxkmRrfmGpQR2Kc66XhG6m3ta")
	if !assert.NoError(t, err) {
		return
	}
	FoundationAddress = *foundation

	chainStore, err := newTestChainStore()
	if err != nil {
		t.Fatal("open LedgerStore err:", err)
		os.Exit(1)
	}

	err = Init(chainStore)
	if err != nil {
		t.Fatal(err, "BlockChain generate failed")
	}

	txPool.Init()
}

func TestTxPool_VerifyDuplicateSidechainTx(t *testing.T) {
	// 1. Generate a withdraw transaction
	txn1 := new(core.Transaction)
	txn1.TxType = core.WithdrawFromSideChain
	txn1.Payload = &core.PayloadWithdrawFromSideChain{
		BlockHeight:         100,
		GenesisBlockAddress: "eb7adb1fea0dd6185b09a43bdcd4924bb22bff7151f0b1b4e08699840ab1384b",
		SideChainTransactionHashes: []string{
			"8a6cb4b5ff1a4f8368c6513a536c663381e3fdeff738e9b437bd8fce3fb30b62",
			"cc62e14f5f9526b7f4ff9d34dcd0643dacb7886707c57f49ec97b95ec5c4edac",
		},
	}

	// 2. Add sidechain Tx to pool
	witPayload := txn1.Payload.(*core.PayloadWithdrawFromSideChain)
	for _, hash := range witPayload.SideChainTransactionHashes {
		success := txPool.addSidechainTx(hash)
		if !success {
			t.Error("Add sidechain Tx to pool failed")
		}
	}

	// 3. Generate a withdraw transaction with duplicate sidechain Tx which already in the pool
	txn2 := new(core.Transaction)
	txn2.TxType = core.WithdrawFromSideChain
	txn2.Payload = &core.PayloadWithdrawFromSideChain{
		BlockHeight:         100,
		GenesisBlockAddress: "eb7adb1fea0dd6185b09a43bdcd4924bb22bff7151f0b1b4e08699840ab1384b",
		SideChainTransactionHashes: []string{
			"8a6cb4b5ff1a4f8368c6513a536c663381e3fdeff738e9b437bd8fce3fb30b62", // duplicate sidechain Tx
		},
	}

	// 4. Run verifyDuplicateSidechainTx
	err := txPool.verifyDuplicateSidechainTx(txn2)
	if err == nil {
		t.Error("Should find the duplicate sidechain tx")
	}
}

func TestTxPool_CleanSidechainTx(t *testing.T) {
	// 1. Generate some withdraw transactions
	txn1 := new(core.Transaction)
	txn1.TxType = core.WithdrawFromSideChain
	txn1.Payload = &core.PayloadWithdrawFromSideChain{
		BlockHeight:         100,
		GenesisBlockAddress: "eb7adb1fea0dd6185b09a43bdcd4924bb22bff7151f0b1b4e08699840ab1384b",
		SideChainTransactionHashes: []string{
			"300db7783393a6f60533c1223108445df57de4fb4842f84f55d07df57caa0c7d",
			"d6c2cb8345a8fe4af0d103cc4e40dbb0654bb169a85bb8cc57923d0c72f3658f",
		},
	}

	txn2 := new(core.Transaction)
	txn2.TxType = core.WithdrawFromSideChain
	txn2.Payload = &core.PayloadWithdrawFromSideChain{
		BlockHeight:         100,
		GenesisBlockAddress: "eb7adb1fea0dd6185b09a43bdcd4924bb22bff7151f0b1b4e08699840ab1384b",
		SideChainTransactionHashes: []string{
			"326218253e6feaa21e3521eff27418b942a5fbd45347505f3e5aca0463baffe2",
		},
	}

	txn3 := new(core.Transaction)
	txn3.TxType = core.WithdrawFromSideChain
	txn3.Payload = &core.PayloadWithdrawFromSideChain{
		BlockHeight:         100,
		GenesisBlockAddress: "eb7adb1fea0dd6185b09a43bdcd4924bb22bff7151f0b1b4e08699840ab1384b",
		SideChainTransactionHashes: []string{
			"645b614eaaa0a1bfd7015d88f3c1343048343924fc105e403b735ba754caa8db",
			"9dcad6d4ec2851bf522ddd301c7567caf98554a82a0bcce866de80b503909642",
		},
	}
	txns := []*core.Transaction{txn1, txn2, txn3}

	// 2. Add to sidechain txs pool
	for _, txn := range txns {
		witPayload := txn.Payload.(*core.PayloadWithdrawFromSideChain)
		for _, hash := range witPayload.SideChainTransactionHashes {
			success := txPool.addSidechainTx(hash)
			if !success {
				t.Error("Add to sidechain tx pool failed")
			}
		}
	}

	// Verify sidechain tx pool state
	for _, txn := range txns {
		err := txPool.verifyDuplicateSidechainTx(txn)
		if err == nil {
			t.Error("Should find the duplicate sidechain tx")
		}
	}

	// 3. Run cleanSidechainTx
	txPool.cleanSidechainTx(txns)

	// Verify sidechian tx pool state
	for _, txn := range txns {
		err := txPool.verifyDuplicateSidechainTx(txn)
		if err != nil {
			t.Error("Should not find the duplicate sidechain tx")
		}
	}
}

func TestTxPool_ReplaceDuplicateSideChainPowTx(t *testing.T) {
	var sideBlockHash1 common.Uint256
	var sideBlockHash2 common.Uint256
	var sideGenesisHash common.Uint256
	rand.Read(sideBlockHash1[:])
	rand.Read(sideBlockHash2[:])
	rand.Read(sideGenesisHash[:])

	txn1 := new(core.Transaction)
	txn1.TxType = core.SideChainPow
	txn1.Payload = &core.PayloadSideChainPow{
		SideBlockHash:   sideBlockHash1,
		SideGenesisHash: sideGenesisHash,
		BlockHeight:     100,
	}

	ok := txPool.addToTxList(txn1)
	if !ok {
		t.Error("Add sidechainpow txn1 to txpool failed")
	}

	txn2 := new(core.Transaction)
	txn2.TxType = core.SideChainPow
	txn2.Payload = &core.PayloadSideChainPow{
		SideBlockHash:   sideBlockHash2,
		SideGenesisHash: sideGenesisHash,
		BlockHeight:     100,
	}
	txPool.replaceDuplicateSideChainPowTx(txn2)
	ok = txPool.addToTxList(txn2)
	if !ok {
		t.Error("Add sidechainpow txn2 to txpool failed")
	}

	if txn := txPool.GetTransaction(txn1.Hash()); txn != nil {
		t.Errorf("Txn1 should be replaced")
	}

	if txn := txPool.GetTransaction(txn2.Hash()); txn == nil {
		t.Errorf("Txn2 should be added in txpool")
	}
}

func TestTxPool_IsDuplicateSidechainTx(t *testing.T) {
	var sideTx1 common.Uint256
	var sideTx2 common.Uint256
	rand.Read(sideTx1[:])
	rand.Read(sideTx2[:])

	// 1. Generate a withdraw transaction
	txn1 := new(core.Transaction)
	txn1.TxType = core.WithdrawFromSideChain
	txn1.Payload = &core.PayloadWithdrawFromSideChain{
		BlockHeight:         100,
		GenesisBlockAddress: "eb7adb1fea0dd6185b09a43bdcd4924bb22bff7151f0b1b4e08699840ab1384b",
		SideChainTransactionHashes: []string{
			sideTx1.String(),
			sideTx2.String(),
		},
	}

	// 2. Add sidechain Tx to pool
	witPayload := txn1.Payload.(*core.PayloadWithdrawFromSideChain)
	for _, hash := range witPayload.SideChainTransactionHashes {
		success := txPool.addSidechainTx(hash)
		if !success {
			t.Error("Add sidechain Tx to pool failed")
		}
	}

	// 3. Run IsDuplicateSidechainTx
	inPool := txPool.IsDuplicateSidechainTx(sideTx1.String())
	if !inPool {
		t.Error("Should find duplicate sidechain tx")
	}
}

func TestTxPool_CleanSubmittedTransactions(t *testing.T) {
	var input *core.Input
	var inputTxID common.Uint256
	inputTxID.Deserialize(bytes.NewReader([]byte("6817addb1eb959d0d56117fd54b6e795788d54ec2a950c209d858da182cf3291")))
	input = &core.Input{
		Previous: core.OutPoint{
			//TxID:  inputTxID,
			Index: 0,
		},
		Sequence: 100,
	}
	//two mock transactions
	tx1 := new(core.Transaction)
	tx1.TxType = core.TransferAsset
	tx1.PayloadVersion = 0
	tx1.Payload = &core.PayloadTransferAsset{}
	var attribute1 *core.Attribute
	attribute1 = &core.Attribute{
		Usage: core.Nonce,
		Data: []byte("5217023ca4139475f8a4c2772a113168568da958c05faaaedff1b3" +
			"77d420ed328f39f15420f48ce4e9c83b69b14e88da00ab6c87f35dc5841c064" +
			"35b7c49dbf3a944171e3d8604dd817324bb2c77f0500000ae0858a6c4222a83" +
			"ba0c42ea3d8038177531a4dfc8183a0ab1de6741e6da79b8bddeacdeeefb78f" +
			"586c8bc45e9c"),
	}
	tx1.Attributes = []*core.Attribute{attribute1}

	tx1.Inputs = []*core.Input{input}
	//var output *core.Output
	//*output = core.Output{AssetID: *assetIDUint256, Value: common.Fixed64(1)}

	tx2 := new(core.Transaction)
	tx2.TxType = core.TransferAsset
	tx2.PayloadVersion = 0
	tx2.Payload = &core.PayloadTransferAsset{}
	var attribute2 *core.Attribute
	attribute2 = &core.Attribute{
		Usage: core.Nonce,
		Data: []byte("202bf0908cfe9687d04f4dc29f3b73eea8d0f7b00d159a3f4843a4" +
			"400a86297404bda1c1f2f5c497149db3fdea371f1bb9e71c86dafccce128944" +
			"b26a7181ebafa9e4869cdfbc7a6e1f34b8818a78f361888907452a05d04c399" +
			"1c10e92b1041e7258611dc52059917f4a946ea89cf68b7af0808e89aa5d8241" +
			"e453410fb1f46"),
	}
	tx2.Attributes = []*core.Attribute{attribute2}
	tx2.Inputs = []*core.Input{input}

	// a mock block
	var newBLock core.Block
	var previousBlockHash common.Uint256
	var merkleRoot common.Uint256
	var blockAuxpow auxpow.AuxPow
	blockAuxpow.Deserialize(bytes.NewReader([]byte("01000000010000000000000000" +
		"000000000000000000000000000000000000000000000000000000002cfabe6d6d0" +
		"5282102a9ced24c5d8260407b8685f57ec3e9485e00a17d9a43d66f90e776aa0100" +
		"0000000000000000000000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000000000000000000ffffff7f000000000000" +
		"000000000000000000000000000000000000000000000000000029a6f8a6f4b265a" +
		"4b96f83a570025c07552480934ca17ccbac69d43db7331bd86229275b0000000003" +
		"000000")))
	previousBlockHash.Deserialize(bytes.NewReader([]byte("5570625560dcd24ceeb8a5758aafd5a66045c159b5b00edcbaec59566b4d65bf")))
	merkleRoot.Deserialize(bytes.NewReader([]byte("0cd26e5ef833e469ed0e0df7cdc7b22f4cf294492c450e677c8a47846afecf22")))
	newBLock.Version = 0
	newBLock.Previous = previousBlockHash
	newBLock.MerkleRoot = merkleRoot
	newBLock.Timestamp = 1529293192
	newBLock.Bits = 545259519
	newBLock.Nonce = 0
	newBLock.Height = 221
	newBLock.AuxPow = blockAuxpow
	newBLock.Transactions = []*core.Transaction{tx2}

	txPool.addToTxList(tx1)
	txPool.addInputUTXOList(tx1, input)

	txPool.CleanSubmittedTransactions(&newBLock)

	tx := txPool.txnList[tx1.Hash()]
	if tx != nil {
		t.Error("Should delete double spent utxo transaction")
	}
}
