package fil_parser

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zondax/fil-parser/parser"

	"go.uber.org/zap"

	"github.com/filecoin-project/lotus/api"
	"github.com/zondax/fil-parser/actors/cache/impl/common"
	v1 "github.com/zondax/fil-parser/parser/v1"
	v2 "github.com/zondax/fil-parser/parser/v2"

	"github.com/bytedance/sonic"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/stretchr/testify/require"
	rosettaFilecoinLib "github.com/zondax/rosetta-filecoin-lib"

	"github.com/zondax/fil-parser/types"
)

const (
	dataPath          = "data/heights"
	fileDataExtension = "json.gz"
	tracesPrefix      = "traces"
	tipsetPrefix      = "tipset"
	ethLogPrefix      = "ethlog"
	nodeUrl           = "https://api.zondax.ch/fil/node/mainnet/rpc/v1"
	calibNextNodeUrl  = "https://node-fil-calibration-next.zondax.ch/rpc/v1"
	feeType           = "fee"
)

func getFilename(prefix, height string) string {
	return fmt.Sprintf(`%s/%s_%s.%s`, dataPath, prefix, height, fileDataExtension)
}

func tracesFilename(height string) string {
	return getFilename(tracesPrefix, height)
}

func ehtlogFilename(height string) string {
	return getFilename(ethLogPrefix, height)
}

func tipsetFilename(height string) string {
	return getFilename(tipsetPrefix, height)
}

func readGzFile(fileName string) ([]byte, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(gzipReader)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readTipset(height string) (*types.ExtendedTipSet, error) {
	rawTipset, err := readGzFile(tipsetFilename(height))
	if err != nil {
		return nil, err
	}
	var tipset *types.ExtendedTipSet
	err = sonic.Unmarshal(rawTipset, &tipset)
	if err != nil {
		return nil, err
	}

	return tipset, nil
}

func readEthLogs(height string) ([]types.EthLog, error) {
	rawLogs, err := readGzFile(ehtlogFilename(height))
	if err != nil {
		return nil, err
	}
	var logs []types.EthLog
	err = sonic.Unmarshal(rawLogs, &logs)
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func getLotusClient(t *testing.T, url string) api.FullNode {
	lotusClient, _, err := client.NewFullNodeRPCV1(context.Background(), url, http.Header{})
	require.NoError(t, err)
	require.NotNil(t, lotusClient, "Lotus client should not be nil")

	return lotusClient
}

func getLib(t *testing.T, nodeURL string) *rosettaFilecoinLib.RosettaConstructionFilecoin {
	lotusClient := getLotusClient(t, nodeURL)

	lib := rosettaFilecoinLib.NewRosettaConstructionFilecoin(lotusClient)
	require.NotNil(t, lib, "Rosetta lib should not be nil")
	return lib
}

func getCacheDataSource(t *testing.T, nodeURL string) common.DataSource {
	return common.DataSource{
		Node: getLotusClient(t, nodeURL),
	}
}

func TestParser_ParseTransactions(t *testing.T) {
	// expectedResults are from previous runs. This assures backward compatibility. (Worst case would be fewer traces
	// or address than previous versions)
	type expectedResults struct {
		totalTraces  int
		totalAddress int
	}
	tests := []struct {
		name    string
		version string
		url     string
		height  string
		results expectedResults
	}{
		{
			name:    "parser with traces from v1",
			version: v1.NodeVersionsSupported[0],
			url:     nodeUrl,
			height:  "2907480",
			results: expectedResults{
				totalTraces:  650,
				totalAddress: 98,
			},
		},
		{
			name:    "parser with traces from v1 and the corner case of duplicated fees with level 0",
			version: v1.NodeVersionsSupported[0],
			url:     nodeUrl,
			height:  "845259",
			results: expectedResults{
				totalTraces:  31,
				totalAddress: 3,
			},
		},
		{
			name:    "parser with traces from v2",
			version: v2.NodeVersionsSupported[0],
			url:     nodeUrl,
			height:  "2907520",
			results: expectedResults{
				totalTraces:  907,
				totalAddress: 88,
			},
		},
		{
			name:    "parser with traces from v2 and lotus 1.25",
			version: v2.NodeVersionsSupported[2],
			url:     nodeUrl,
			height:  "3573062",
			results: expectedResults{
				totalTraces:  773,
				totalAddress: 70,
			},
		},
		{
			name:    "parser with traces from v2 and lotus 1.25",
			version: v2.NodeVersionsSupported[2],
			url:     nodeUrl,
			height:  "3573064",
			results: expectedResults{
				totalTraces:  734,
				totalAddress: 75,
			},
		},
		{
			name:    "parser with traces from v2 and lotus 1.25",
			version: v2.NodeVersionsSupported[2],
			url:     nodeUrl,
			height:  "3573066",
			results: expectedResults{
				totalTraces:  1118,
				totalAddress: 102,
			},
		},
		{
			name:    "parser with traces from v2 and lotus 1.26 (calib)",
			version: v2.NodeVersionsSupported[2],
			url:     calibNextNodeUrl,
			height:  "1419335",
			results: expectedResults{
				totalTraces:  37,
				totalAddress: 11,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := getLib(t, tt.url)

			tipset, err := readTipset(tt.height)
			require.NoError(t, err)
			ethlogs, err := readEthLogs(tt.height)
			require.NoError(t, err)
			traces, err := readGzFile(tracesFilename(tt.height))
			require.NoError(t, err)

			logger, err := zap.NewDevelopment()
			require.NoError(t, err)

			p, err := NewFilecoinParser(lib, getCacheDataSource(t, tt.url), logger)
			require.NoError(t, err)
			txs, adds, err := p.ParseTransactions(traces, tipset, ethlogs, types.BlockMetadata{NodeInfo: types.NodeInfo{NodeMajorMinorVersion: tt.version}})
			require.NoError(t, err)
			require.NotNil(t, txs)
			require.NotNil(t, adds)
			require.Equal(t, tt.results.totalTraces, len(txs))
			require.Equal(t, tt.results.totalAddress, adds.Len())
		})
	}
}

func TestParser_GetBaseFee(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		url      string
		height   string
		baseFee  *big.Int
		fallback bool
	}{
		{
			name:    "parser with getBaseFee",
			version: v2.NodeVersionsSupported[0],
			url:     nodeUrl,
			height:  "2907480",
			baseFee: big.NewInt(96036633),
		},
		{
			name:     "parser with getBaseFee fallback",
			version:  v2.NodeVersionsSupported[0],
			url:      nodeUrl,
			height:   "3450305",
			baseFee:  big.NewInt(100),
			fallback: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := getLib(t, tt.url)

			tipset, err := readTipset(tt.height)
			require.NoError(t, err)
			traces, err := readGzFile(tracesFilename(tt.height))
			require.NoError(t, err)

			logger, err := zap.NewDevelopment()
			require.NoError(t, err)

			p, err := NewFilecoinParser(lib, getCacheDataSource(t, tt.url), logger)
			require.NoError(t, err)
			baseFee, err := p.GetBaseFee(traces, types.BlockMetadata{}, tipset)
			require.NoError(t, err)
			require.Equal(t, baseFee, tt.baseFee.Uint64())
			if tt.fallback {
				require.Equal(t, baseFee, tipset.Blocks()[0].ParentBaseFee.Uint64())
			}
		})
	}
}

func TestParser_InDepthCompare(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		height string
	}{
		{
			name:   "height downloaded with v1",
			url:    nodeUrl,
			height: "2907480",
		},
		{
			name:   "height downloaded with v2",
			url:    nodeUrl,
			height: "2907520",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := getLib(t, tt.url)

			tipset, err := readTipset(tt.height)
			require.NoError(t, err)
			ethlogs, err := readEthLogs(tt.height)
			require.NoError(t, err)
			traces, err := readGzFile(tracesFilename(tt.height))
			require.NoError(t, err)

			logger, err := zap.NewDevelopment()
			require.NoError(t, err)

			p, err := NewFilecoinParser(lib, getCacheDataSource(t, tt.url), logger)
			require.NoError(t, err)
			v1Txs, v1Adds, err := p.ParseTransactions(traces, tipset, ethlogs, types.BlockMetadata{NodeInfo: types.NodeInfo{NodeMajorMinorVersion: "v1.22"}})
			require.NoError(t, err)
			require.NotNil(t, v1Txs)
			require.NotNil(t, v1Adds)

			v2Txs, v2Adds, err := p.ParseTransactions(traces, tipset, ethlogs, types.BlockMetadata{NodeInfo: types.NodeInfo{NodeMajorMinorVersion: "v1.23"}})
			require.NoError(t, err)
			require.NotNil(t, v2Txs)
			require.NotNil(t, v2Adds)

			require.Equal(t, len(v1Txs), len(v2Txs))
			require.Equal(t, v1Adds.Len(), v2Adds.Len())

			for i := range v1Txs {
				require.True(t, v1Txs[i].Equal(*v2Txs[i]))
			}

			v1Adds.Range(func(key string, value *types.AddressInfo) bool {
				v2Value, ok := v2Adds.Get(key)
				require.True(t, ok)
				require.Equal(t, value, v2Value)
				return true
			})
		})
	}
}

func TestParseGenesis(t *testing.T) {
	network := "mainnet"
	genesisBalances, genesisTipset, err := getStoredGenesisData(network)
	if err != nil {
		t.Fatalf("Error getting genesis data: %s", err)
	}

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	lib := getLib(t, nodeUrl)
	config := &parser.FilecoinParserConfig{
		ConsolidateAddressesToRobust: parser.ConsolidateAddressesToRobust{
			Enable:     true,
			BestEffort: true,
		},
	}
	p, err := NewFilecoinParser(lib, getCacheDataSource(t, nodeUrl), logger, config)
	assert.NoError(t, err)
	actualTxs, _ := p.ParseGenesis(genesisBalances, genesisTipset)

	assert.Equal(t, len(actualTxs), 21)
	assert.Equal(t, actualTxs[0].BlockCid, "bafy2bzacecnamqgqmifpluoeldx7zzglxcljo6oja4vrmtj7432rphldpdmm2")
	assert.Equal(t, actualTxs[0].TipsetCid, "bafy2bzacea3l7hchfijz5fvswab36fxepf6oagecp5hrstmol7zpm2l4tedf6")
}

func getStoredGenesisData(network string) (*types.GenesisBalances, *types.ExtendedTipSet, error) {
	balancesFilePath := filepath.Join("./data/genesis", fmt.Sprintf("%s_genesis_balances.json", network))
	tipsetFilePath := filepath.Join("./data/genesis", fmt.Sprintf("%s_genesis_tipset.json", network))

	var balances types.GenesisBalances
	var tipset types.ExtendedTipSet

	balancesFileContent, err := os.ReadFile(balancesFilePath)
	if err != nil {
		zap.S().Errorf("Error reading file '%s': %s", balancesFilePath, err.Error())
		return nil, nil, err
	}

	err = json.Unmarshal(balancesFileContent, &balances)
	if err != nil {
		zap.S().Errorf("Error unmarshalling genesis balances: %s", err.Error())
		return nil, nil, err
	}

	tipsetFileContent, err := os.ReadFile(tipsetFilePath)
	if err != nil {
		zap.S().Errorf("Error reading file '%s': %s", tipsetFilePath, err.Error())
		return nil, nil, err
	}

	err = json.Unmarshal(tipsetFileContent, &tipset)
	if err != nil {
		zap.S().Errorf("Error unmarshalling genesis tipset: %s", err.Error())
		return nil, nil, err
	}

	return &balances, &tipset, nil
}
