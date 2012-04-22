package main

import (
	"fmt"
	"net"
	"time"
)

func ResolveAddrs(servers []string) ([]*net.UDPAddr, error) {
	res := make([]*net.UDPAddr, 0, len(servers))
	for _, s := range servers {
		addr, err := net.ResolveUDPAddr("udp", s)
		if err != nil {
			return nil, err
		}
		res = append(res, addr)
	}
	return res, nil
}

type Result struct {
	UTC, Local    time.Time
	NumOK, NumBad int
}

func (r Result) Bad() bool {
	return r.NumBad >= r.NumOK
}

func AttemptOne(server *net.UDPAddr, ch chan bool) {
	conn, err := net.Dial("udp", server.String())
	res := false
	defer func() {
		ch <- res
	}()
	if err != nil {
		return
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return
	}

	request := []byte{
		0, 1, // Binding request
		0, 0, // Message length
		0x21, 0x12, 0xa4, 0x42, // magic
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12} // TID

	if n, err := conn.Write(request); err != nil || n != len(request) {
		return
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	res = (n > 0 && err == nil)
	return
}

func Attempt(servers []*net.UDPAddr) Result {
	ch := make(chan bool, len(servers))
	for _, server := range servers {
		go AttemptOne(server, ch)
	}
	res := Result{UTC: time.Now().UTC(), Local: time.Now()}
	for _ = range servers {
		if <-ch {
			res.NumOK++
		} else {
			res.NumBad++
		}
	}
	return res
}

func main() {
	servers := []string{
		"stun.l.google.com:19302",
		"stun.ekiga.net:3478",
		"stunserver.org:3478",
		"stun.xten.com:3478",
		"stun01.sipphone.com:3478",
		"stun.softjoys.com:3478",
		"stun.voxgratia.org:3478",
		"stun1.noc.ams-ix.net:3478"}

	addrs, err := ResolveAddrs(servers)
	if err != nil {
		fmt.Println(err)
	}
	for i := range servers {
		fmt.Println(servers[i], addrs[i])
	}

	ticker := time.Tick(4e9)
	up := true
	var last *Result = nil
	for {
		now := Attempt(addrs)
		if now.Bad() != up {
			if now.Bad() {
				fmt.Printf("DOWN %+v\n", now)
			} else {
				fmt.Printf("UP   %+v\n", now)
				if last != nil {
					fmt.Printf("OUTAGE %s %d seconds\n", last.Local, now.Local.Unix()-last.Local.Unix())
				}
			}
			last = &now
		} else {
			fmt.Printf("NOP  %+v\n", now)
		}
		up = now.Bad()
		<-ticker
	}
}
