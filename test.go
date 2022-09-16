package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	 "crypto/ecdsa"

	 swarm "github.com/libp2p/go-libp2p-swarm"
	 "github.com/libp2p/go-libp2p-core/network"
	 ethCrypto "github.com/ethereum/go-ethereum/crypto"
	 libp2p "github.com/libp2p/go-libp2p"
	 p2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
//	mrand "math/rand"
//	"crypto/rand"

//	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/ipfs/go-log/v2"
	relay "github.com/libp2p/go-libp2p-circuit"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
)

var logger = log.Logger("group-chat")
const topic = "sylo-group-chat-demo"

type message struct {
	Clock uint
	ID    string
	Name  string
	Text  string
}

type messageLog struct {
	mu    sync.Mutex
	data  map[message]struct{}
	clock uint
}
func errCheck(err error){
	if err != nil{
		panic(err)
	}
}
func handleStream(stream network.Stream) {
	fmt.Println("Got a new stream!")

	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	go readData(rw)
	go writeData(rw)

	// 'stream' will stay open until you close it (or the other side closes it).
}
func readData(rw *bufio.ReadWriter) {
	for {
		str, err := rw.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from buffer")
			panic(err)
		}

		if str == "" {
			return
		}
		if str != "\n" {
			fmt.Printf("\x1b[32m%s\x1b[0m>> ",str)
		}

	}
}

func writeData(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(">> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from stdin")
			panic(err)
		}

		_, err = rw.WriteString(fmt.Sprintf("%s\n", sendData))
		if err != nil {
			fmt.Println("Error writing to buffer")
			panic(err)
		}
		err = rw.Flush()
		if err != nil {
			fmt.Println("Error flushing buffer")
			panic(err)
		}
	}
}
type LibP2PIdentity struct {
	PrivateKey     p2pCrypto.PrivKey
	PublicKey      p2pCrypto.PubKey
	ID             peer.ID
	IdentityOption libp2p.Option // Used for host creation using this identity
}

func NewLibP2PIdentityFromEthPrivKey(ethPrivKey *ecdsa.PrivateKey) (wallet LibP2PIdentity, err error) {
	privKeyData := ethCrypto.FromECDSA(ethPrivKey)
	privKey, err := p2pCrypto.UnmarshalSecp256k1PrivateKey(privKeyData)
	if err != nil {
		return
	}
	pubKey := privKey.GetPublic()
	id, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return
	}
	wallet = LibP2PIdentity{
		PrivateKey:     privKey,
		PublicKey:      pubKey,
		ID:             id,
		IdentityOption: libp2p.Identity(privKey),
	}
	return
}

func P2PIDFromEthPubKey(ethPubKey *ecdsa.PublicKey) (id peer.ID, err error) {
	pubKeyData := ethCrypto.FromECDSAPub(ethPubKey)
	pubKey, err := p2pCrypto.UnmarshalSecp256k1PublicKey(pubKeyData)
	if err != nil {
		return
	}
	id, err = peer.IDFromPublicKey(pubKey)
	return
}
func (log *messageLog) Append(m message) {
	log.mu.Lock()
	defer log.mu.Unlock()
	if _, ok := log.data[m]; ok {
		// we already have this message
		return
	}
	name := m.Name
	if name == "" {
		name = m.ID[len(m.ID)-6 : len(m.ID)]
	}
	logger.Infof("%s:\t%s", name, m.Text)
	if m.Clock >= log.clock {
		log.clock = m.Clock + 1
	}
}

type bootstraps []*peer.AddrInfo

func (bs *bootstraps) String() string {
	strs := make([]string, len(*bs))
	for i, addr := range *bs {
		strs[i] = addr.String()
	}
	return strings.Join(strs, ",")
}

func (bs *bootstraps) Set(str string) error {
	addr, err := multiaddr.NewMultiaddr(str)
	if err != nil {
		return err
	}
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return err
	}
	*bs = append(*bs, info)
	return nil
}

func main() {
	_ = log.SetLogLevel("group-chat", "info")

	// parse command line arguments
	var connect bool
	var dialID string

	var bs bootstraps
	var name string
	var hop bool
	var ro bool
	var ip string
	var privKeyHex string
	// var privKeyHex string = "1e21018070ceefa36c774ffb16e6eda246e3b89537874397a87357399189210f"

	flag.StringVar(&privKeyHex, "priv", "", "Replace Ether Key with libp2p PeerID")
	flag.BoolVar(&connect, "connect", false, "first stream")
	flag.StringVar(&dialID, "dial", "", "diali Node ID")

	flag.Var(&bs, "bootstrap", "will connect to this `PEER` to bootstrap the network")
	flag.StringVar(&name, "nickname", "", "this `NAME` will be attached to your messages")
	flag.BoolVar(&hop, "relay", false, "allows other peers to relay through this peer")
	flag.BoolVar(&ro, "read-only", false, "disable input and just observe the chat")
	flag.StringVar(&ip, "ip", "", "public `IP` address (for relay peers)")
	flag.Parse()
	privKey, err := ethCrypto.HexToECDSA(privKeyHex)
	if err != nil{
		panic(err)
	}
	wallet,err := NewLibP2PIdentityFromEthPrivKey(privKey)
	if err != nil{
		panic(err)
	}
	/*id, err := P2PIDFromEthPubKey(&privKey.PublicKey)
	if err != nil{
		panic(err)
	}*/

	if hop && ip == "" {
		logger.Fatal("a public ip address is required when starting as a relay")
	}

	// create message log
	log := messageLog{}
	log.data = make(map[message]struct{})

	// start libp2p host
	ctx := context.Background()
	enableRelay := libp2p.EnableRelay()
	if hop {
		enableRelay = libp2p.EnableRelay(relay.OptHop)
	}
	h, err := libp2p.New(ctx,
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) { return dht.New(ctx, h) }),
		libp2p.EnableNATService(),
		libp2p.Identity(wallet.PrivateKey),
		enableRelay,
		libp2p.EnableAutoRelay(),
	)
	//h.Addrs()[0]
	fmt.Println("Addr:",h.ID())
	if err != nil {
		logger.Fatalf("could not start libp2p host: %v", err)
	}

	// connect to bootstrap peers
	for _, b := range bs {
		if err := h.Connect(ctx, *b); err != nil {
			logger.Errorf("could not connect to bootstrap node: %v", err)
			continue
		}
	}
	if hop {
		fmt.Println("relay peers must wait 1 minutes before use")
		time.Sleep(1 * time.Minute)
		fmt.Println("ready to go")
		for _, a := range h.Addrs() {
			if strings.Contains(a.String(), "127.0.0.1") {
				fmt.Printf("%v/p2p/%v\n", strings.Replace(a.String(), "127.0.0.1", ip, 1), h.ID())
			}
		}
	}
	if ro {
		// read only
		<-ctx.Done()
		return
	}

	if connect && dialID == "" {
		fmt.Println("A")
		h.SetStreamHandler("/example/chat/0.1.0", func(s network.Stream){
			rw := bufio. NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
			fmt.Println("---Start---")
			go readData(rw)
			go writeData(rw)
		})
	}
	if dialID != ""{
		fmt.Println("B")
		dialNodeID, err := peer.IDB58Decode(dialID)
		errCheck(err)
		fmt.Println("dial", dialNodeID)
		_, err = h.NewStream(context.Background(), dialNodeID, "/example/chat/0.1.0")
		if err == nil {
			fmt.Println("worked")
				return
		}
		fmt.Println("No connection")
		h.Network().(*swarm.Swarm).Backoff().Clear(dialNodeID)
		multiaddr.SwapToP2pMultiaddrs()
		relayaddr, err := multiaddr.NewMultiaddr("/ip4/115.85.181.212/tcp/30000/p2p/16Uiu2HAmB25j29wr77zLBqbZrEkRvXa4KG3SB5hgvrXfE4KgipE5/p2p-circuit/p2p/"+dialNodeID.Pretty())
		errCheck(err)
		relayInfo := peer.AddrInfo{
			ID: dialNodeID,
			Addrs: []multiaddr.Multiaddr{relayaddr},
		}
		
		if err:= h.Connect(context.Background(), relayInfo); err != nil{
			panic(err)
		}
		s, err := h.NewStream(context.Background(), dialNodeID, "/example/chat/0.1.0")
		if err != nil {
			fmt.Println("worked")
			panic(err)
			return
		}else {
			fmt.Println("connected to: ", relayInfo.ID)
			rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))
			go writeData(rw)
			go readData(rw)
		}

	}

	select{}
}
