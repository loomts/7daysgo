package cache

import (
	"7daysgo/cache/consistent_hash"
	"7daysgo/cache/pb"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
	"sync"
	"time"
)

type GrpcPool struct {
	pb.UnimplementedGroupCacheServer
	me          string
	mu          sync.Mutex
	peers       *consistent_hash.Map
	grpcGetters map[string]*grpcGetter
}

type grpcGetter struct {
	addr string
}

func MakeGrpcPool(me string) *GrpcPool {
	return &GrpcPool{
		me: me,
	}
}

func (g *grpcGetter) Get(req *pb.Request, resp *pb.Response) error {
	conn, err := grpc.Dial(g.addr, grpc.WithTransportCredentials(insecure.NewCredentials())) // 建立一个安全连接
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewGroupCacheClient(conn)
	fmt.Printf("get %v %v from %v\n", req.GetKey(), req.GetGroup(), g.addr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := client.Get(ctx, req)
	fmt.Printf("get %v %v from %v\n", req.GetKey(), req.GetGroup(), g.addr)
	resp.Value = response.Value
	return err
}

func (p *GrpcPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistent_hash.New(replicas, nil)
	p.peers.Add(peers...)
	p.grpcGetters = map[string]*grpcGetter{}
	for _, peer := range peers {
		p.grpcGetters[peer] = &grpcGetter{addr: peer}
	}
}

func (p *GrpcPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.me {
		return p.grpcGetters[peer], true
	}
	return nil, false
}

func (p *GrpcPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.me, fmt.Sprintf(format, v...))
}

func (p *GrpcPool) Get(_ context.Context, req *pb.Request) (*pb.Response, error) {
	p.Log("%s %s", req.Group, req.Key)
	response := &pb.Response{}

	group := GetGroup(req.Group)
	if group == nil {
		p.Log("no such group %v", req.Group)
		return response, fmt.Errorf("no such group %v", req.Group)
	}
	value, err := group.Get(req.Key)
	if err != nil {
		p.Log("get key %v error %v", req.Key, err)
		return response, err
	}

	response.Value = value.ByteSlice()
	return response, nil
}

func (p *GrpcPool) Run() {
	lis, err := net.Listen("tcp", p.me)
	if err != nil {
		panic(err)
	}

	server := grpc.NewServer()
	pb.RegisterGroupCacheServer(server, p)
	reflection.Register(server)
	err = server.Serve(lis)
	if err != nil {
		panic(err)
	}
}

var _ PeerPicker = (*GrpcPool)(nil)
var _ PeerGetter = (*grpcGetter)(nil)
