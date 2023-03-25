package cache

import "7daysgo/cache/pb"

type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

type PeerGetter interface {
	Get(req *pb.Request, resp *pb.Response) error
}
