package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"speedlimitfree/internal/contracts"
	"sync"
	"time"
)

const maxMessage = 4 << 20

type Handler interface {
	Snapshot() contracts.Snapshot
	Command(contracts.Request) error
}

func Serve(ctx context.Context, owner string, handler Handler) error {
	return ServeReady(ctx, owner, handler, nil)
}

// ServeReady reports readiness only after the control pipe has been opened.
func ServeReady(ctx context.Context, owner string, handler Handler, ready func()) error {
	sid, err := windows.StringToSid(owner)
	if err != nil {
		return fmt.Errorf("invalid owner SID: %w", err)
	}
	listener, err := winio.ListenPipe(contracts.PipeName, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + sid.String() + ")", InputBufferSize: 65536, OutputBufferSize: 65536})
	if err != nil {
		return err
	}
	defer listener.Close()
	if ready != nil {
		ready()
	}
	go func() { <-ctx.Done(); listener.Close() }()
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, 16)
	for {
		c, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			c.Close()
			continue
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			defer func() { <-slots }()
			defer c.Close()
			c.SetDeadline(time.Now().Add(5 * time.Second))
			d := json.NewDecoder(io.LimitReader(c, 65536))
			d.DisallowUnknownFields()
			var req contracts.Request
			res := contracts.Response{Version: contracts.Version}
			if err := d.Decode(&req); err != nil {
				res.Error = "invalid request"
			} else if req.Version != contracts.Version {
				res.Error = "desktop/service protocol versions differ"
			} else if req.Method != "snapshot" {
				if err := handler.Command(req); err != nil {
					res.Error = err.Error()
				}
			}
			if res.Error == "" {
				s := handler.Snapshot()
				res.Snapshot = &s
			}
			data, err := json.Marshal(res)
			if err != nil || len(data) > maxMessage {
				data = []byte(`{"version":1,"error":"service response exceeds size limit"}`)
			}
			c.Write(append(data, '\n'))
		}(c)
	}
}

func Call(ctx context.Context, req contracts.Request) (contracts.Snapshot, error) {
	req.Version = contracts.Version
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, contracts.PipeName)
	if err != nil {
		return contracts.Snapshot{}, fmt.Errorf("background service is not connected: %w", err)
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	c.SetDeadline(deadline)
	if err = json.NewEncoder(c).Encode(req); err != nil {
		return contracts.Snapshot{}, err
	}
	var res contracts.Response
	if err = json.NewDecoder(io.LimitReader(c, maxMessage)).Decode(&res); err != nil {
		return contracts.Snapshot{}, err
	}
	if res.Version != contracts.Version {
		return contracts.Snapshot{}, errors.New("desktop/service protocol versions differ")
	}
	if res.Error != "" {
		return contracts.Snapshot{}, errors.New(res.Error)
	}
	if res.Snapshot == nil {
		return contracts.Snapshot{}, errors.New("service returned no snapshot")
	}
	return *res.Snapshot, nil
}
