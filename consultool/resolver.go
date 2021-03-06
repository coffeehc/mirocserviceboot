package consultool

import (
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/coffeehc/logger"
	"github.com/coffeehc/microserviceboot/base"
	"github.com/coffeehc/microserviceboot/loadbalancer"
	"github.com/hashicorp/consul/api"
	"google.golang.org/grpc/naming"
)

type _ConsulResolver struct {
	c           *api.Client
	service     string
	tag         string
	passingOnly bool

	quitc       chan struct{}
	quitUpdate  chan struct{}
	updatesc    chan []*naming.Update
	updateMutex *sync.Mutex
}

// NewConsulResolver initializes and returns a new ConsulResolver.
//
// It resolves addresses for gRPC connections to the given service and tag.
// If the tag is irrelevant, use an empty string.
func newConsulResolver(client *api.Client, service, tag string) (naming.Resolver, base.Error) {
	r := &_ConsulResolver{
		c:           client,
		service:     service,
		tag:         tag,
		passingOnly: true,
		quitc:       make(chan struct{}),
		quitUpdate:  make(chan struct{}),
		updatesc:    make(chan []*naming.Update, 1),
		updateMutex: new(sync.Mutex),
	}

	// Retrieve instances immediately
	instancesCh := make(chan []string)
	go func() {
		sleep := int64(time.Second * 10)
		for {
			instances, _, err := r.getInstances(0)
			if err != nil {
				logger.Warn("lb: error retrieving instances from Consul: %v", err)
				time.Sleep(time.Duration(rand.Int63n(sleep)))
				continue
			}
			instancesCh <- instances
			return
		}
	}()
	instances := <-instancesCh
	r.updatesc <- r.makeUpdates(nil, instances)
	// Start updater
	go r.updater(instances, 0)
	return r, nil
}

// Resolve creates a watcher for target. The watcher interface is implemented
// by ConsulResolver as well, see Next and Close.
func (r *_ConsulResolver) Resolve(target string) (naming.Watcher, error) {
	return r, nil
}

// Next blocks until an update or error happens. It may return one or more
// updates. The first call will return the full set of instances available
// as NewConsulResolver will look those up. Subsequent calls to Next() will
// block until the resolver finds any new or removed instance.
//
// An error is returned if and only if the watcher cannot recover.
func (r *_ConsulResolver) Next() ([]*naming.Update, error) {
	return <-r.updatesc, nil
}

// Close closes the watcher.
func (r *_ConsulResolver) Close() {
	select {
	case <-r.quitc:
	default:
		close(r.quitc)
		close(r.updatesc)
	}
}

// updater is a background process started in NewConsulResolver. It takes
// a list of previously resolved instances (in the format of host:port, e.g.
// 192.168.0.1:1234) and the last index returned from Consul.
func (r *_ConsulResolver) updater(instances []string, lastIndex uint64) {
	var err error
	var oldInstances = instances
	var newInstances []string

	// TODO Cache the updates for a while, so that we don't overwhelm Consul.
	sleep := int64(time.Second * 10)
	for {
		select {
		case <-r.quitc:
			break
		case <-r.quitUpdate:
			return
		default:
			func() {
				defer func() {
					if err := recover(); err != nil {
						logger.Warn("update addrs error :%s", err)
					}
				}()
				newInstances, lastIndex, err = r.getInstances(lastIndex)
				if err != nil {
					logger.Warn("lb: error retrieving instances from Consul: %v", err)
					time.Sleep(time.Duration(rand.Int63n(sleep)))
					return
				}
				updates := r.makeUpdates(oldInstances, newInstances)
				if updates == nil || len(updates) == 0 {
					return
				}
				r.updatesc <- updates
				oldInstances = newInstances
			}()
		}
	}
}

// getInstances retrieves the new set of instances registered for the
// service from Consul.
func (r *_ConsulResolver) getInstances(lastIndex uint64) ([]string, uint64, error) {
	services, meta, err := r.c.Health().Service(r.service, r.tag, r.passingOnly, &api.QueryOptions{
		WaitIndex: lastIndex,
		WaitTime:  time.Second,
	})
	if err != nil {
		return nil, lastIndex, err
	}
	if len(services) == 0 {
		return nil, lastIndex, base.NewError(base.Error_System, "consul resolver", "service is no address available")
	}
	var instances []string
	for _, service := range services {
		s := service.Service.Address
		if len(s) == 0 {
			s = service.Node.Address
		}
		addr := net.JoinHostPort(s, strconv.Itoa(service.Service.Port))
		instances = append(instances, addr)
	}
	return instances, meta.LastIndex, nil
}

// makeUpdates calculates the difference between and old and a new set of
// instances and turns it into an array of naming.Updates.
func (r *_ConsulResolver) makeUpdates(oldInstances, newInstances []string) []*naming.Update {
	oldAddr := make(map[string]struct{}, len(oldInstances))
	for _, instance := range oldInstances {
		oldAddr[instance] = struct{}{}
	}
	newAddr := make(map[string]struct{}, len(newInstances))
	for _, instance := range newInstances {
		newAddr[instance] = struct{}{}
	}
	var updates []*naming.Update
	for addr := range newAddr {
		if _, ok := oldAddr[addr]; !ok {
			updates = append(updates, &naming.Update{Op: naming.Add, Addr: addr})
		}
	}
	for addr := range oldAddr {
		if _, ok := newAddr[addr]; !ok {
			updates = append(updates, &naming.Update{Op: naming.Delete, Addr: addr})
		}
	}
	return updates
}

func (sr *_ConsulResolver) Delete(addr loadbalancer.Address) {
	sr.updateMutex.Lock()
	defer sr.updateMutex.Unlock()
	logger.Warn("delete addr [%s]", addr.Addr)
	sr.updatesc <- []*naming.Update{&naming.Update{Op: naming.Delete, Addr: addr.Addr}}
	sr.quitUpdate <- struct{}{}
	time.Sleep(time.Second * 10)
	instances := make([]string, 0)
	go sr.updater(instances, 0)
}
