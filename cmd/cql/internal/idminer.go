/*
 * Copyright 2018-2019 The CovenantSQL Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package internal

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/SQLess/SQLess/crypto/asymmetric"
	"github.com/SQLess/SQLess/crypto/kms"
	mine "github.com/SQLess/SQLess/pow/cpuminer"
	"github.com/SQLess/SQLess/proto"
	"github.com/SQLess/SQLess/utils"
)

var (
	difficulty int
	loop       bool
)

// CmdIDMiner is cql idminer command entity.
var CmdIDMiner = &Command{
	UsageLine: "cql idminer [common params] [-difficulty number] [-loop [true]]",
	Short:     "calculate nonce and node id for config.yaml file",
	Long: `
IDMiner calculates legal node id and it's nonce. Default parameters are difficulty of 24 and
no endless loop.
e.g.
    cql idminer -difficulty 24

If you want mining a good id, use:
    cql idminer -config ~/.cql/config.yaml -loop -difficulty 24
`,
	Flag:       flag.NewFlagSet("IDMiner params", flag.ExitOnError),
	CommonFlag: flag.NewFlagSet("Common params", flag.ExitOnError),
	DebugFlag:  flag.NewFlagSet("Debug params", flag.ExitOnError),
}

func init() {
	CmdIDMiner.Run = runIDMiner

	addCommonFlags(CmdIDMiner)
	addConfigFlag(CmdIDMiner)
	CmdIDMiner.Flag.IntVar(&difficulty, "difficulty", 24, "Difficulty for miner to mine nodes and generating nonce")
	CmdIDMiner.Flag.BoolVar(&loop, "loop", false, "Keep mining until interrupted")
}

func runIDMiner(cmd *Command, args []string) {
	publicKey := getPublicFromConfig()

	if loop {
		nonceLoop(publicKey)
	} else {
		_ = nonceGen(publicKey)
	}
}

func nonceLoop(publicKey *asymmetric.PublicKey) {
	cpuCount := runtime.NumCPU()
	ConsoleLog.Infof("cpu: %#v\n", cpuCount)
	nonceChs := make([]chan mine.NonceInfo, cpuCount)
	stopChs := make([]chan struct{}, cpuCount)

	rand.Seed(time.Now().UnixNano())
	step := math.MaxUint64 / uint64(cpuCount)

	for i := 0; i < cpuCount; i++ {
		nonceChs[i] = make(chan mine.NonceInfo)
		stopChs[i] = make(chan struct{})
		go func(i int) {
			miner := mine.NewCPUMiner(stopChs[i])
			nonceCh := nonceChs[i]
			block := mine.MiningBlock{
				Data:      publicKey.Serialize(),
				NonceChan: nonceCh,
				Stop:      nil,
			}
			start := mine.Uint256{D: step*uint64(i) + uint64(rand.Uint32())}
			ConsoleLog.Infof("miner #%#v start: %#v\n", i, start)
			miner.ComputeBlockNonce(block, start, difficulty)
			//TODO(laodouya) add wait group
		}(i)
	}

	sig := <-utils.WaitForExit()
	ConsoleLog.Infof("received signal %#v\n", sig)
	for i := 0; i < cpuCount; i++ {
		close(stopChs[i])
	}

	max := mine.NonceInfo{}
	for i := 0; i < cpuCount; i++ {
		newNonce := <-nonceChs[i]
		if max.Difficulty < newNonce.Difficulty {
			max = newNonce
		}
	}

	// verify result
	ConsoleLog.Infof("verify result: %#v\n", kms.IsIDPubNonceValid(&proto.RawNodeID{Hash: max.Hash}, &max.Nonce, publicKey))

	// print result
	fmt.Printf("nonce: %v\n", max)
	fmt.Printf("node id: %v\n", max.Hash.String())
}

func nonceGen(publicKey *asymmetric.PublicKey) *mine.NonceInfo {
	publicKeyBytes := publicKey.Serialize()

	cpuCount := runtime.NumCPU()
	ConsoleLog.Infof("cpu: %#v\n", cpuCount)
	stopCh := make(chan struct{})
	nonceCh := make(chan mine.NonceInfo, cpuCount)
	progressCh := make(chan int, 100)
	var wg sync.WaitGroup

	rand.Seed(time.Now().UnixNano())
	step := 256 / cpuCount
	for i := 0; i < cpuCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			startBit := i * step
			position := startBit / 64
			shift := uint(startBit % 64)
			ConsoleLog.Debugf("position: %#v, shift: %#v, i: %#v", position, shift, i)
			var start mine.Uint256
			if position == 0 {
				start = mine.Uint256{A: uint64(1<<shift) + uint64(rand.Uint32())}
			} else if position == 1 {
				start = mine.Uint256{B: uint64(1<<shift) + uint64(rand.Uint32())}
			} else if position == 2 {
				start = mine.Uint256{C: uint64(1<<shift) + uint64(rand.Uint32())}
			} else if position == 3 {
				start = mine.Uint256{D: uint64(1<<shift) + uint64(rand.Uint32())}
			}

			for j := start; ; j.Inc() {
				select {
				case <-stopCh:
					return
				default:
					currentHash := mine.HashBlock(publicKeyBytes, j)
					currentDifficulty := currentHash.Difficulty()
					progressCh <- currentDifficulty
					if currentDifficulty >= difficulty {
						nonce := mine.NonceInfo{
							Nonce:      j,
							Difficulty: currentDifficulty,
							Hash:       currentHash,
						}
						nonceCh <- nonce
						return
					}
				}
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		var count, current int

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case mined := <-progressCh:
				if mined > current {
					current = mined
					fmt.Printf("\rnonce mining %v seconds, current difficulty: %v, target difficulty: %v", count, current, difficulty)
				}
			case <-ticker.C:
				count++
				fmt.Printf("\rnonce mining %v seconds, current difficulty: %v, target difficulty: %v", count, current, difficulty)
			}
		}
	}()

	nonce := <-nonceCh
	close(stopCh)
	wg.Wait()
	fmt.Printf("\n")

	// verify result
	if !kms.IsIDPubNonceValid(&proto.RawNodeID{Hash: nonce.Hash}, &nonce.Nonce, publicKey) {
		ConsoleLog.WithFields(logrus.Fields{
			"nonce": nonce,
			"id":    nonce.Hash.String(),
		}).Fatal("invalid nonce")
	}

	// print result
	fmt.Printf("nonce: %v\n", nonce)
	fmt.Printf("node id: %v\n", nonce.Hash.String())

	return &nonce
}
