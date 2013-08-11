package main

import (
	"expvar"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

var (
	delaySec        = flag.Int("delaySec", 60, "delay between lookup attempts")
	printNoChange   = flag.Bool("printNoChange", false, "print when there's no change")
	listenPort      = flag.Int("port", 0, "port (if any) to export variables using expvar")
	retryDnsForever = flag.Bool("retryDnsForever", true, "keep retrying DNS until all lookups succeed")

	numServers = expvar.NewInt("numServers")
	numVisible = expvar.NewInt("numVisible")
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
	flag.Parse()
	servers := []string{
		"stun.l.google.com:19302",
		"stun.ekiga.net:3478",
		"stunserver.org:3478",
		"stun.xten.com:3478",
		//		"stun01.sipphone.com:3478",
		"stun.softjoys.com:3478",
		"stun.voxgratia.org:3478",
		"stun1.noc.ams-ix.net:3478"}
	numServers.Set(int64(len(servers)))
	if *listenPort > 0 {
		numVisible.Set(0)
		go func() {
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *listenPort), nil))
		}()
	}

	var (
		addrs []*net.UDPAddr
		err   error
	)
	for len(addrs) == 0 {
		addrs, err = ResolveAddrs(servers)
		if err != nil {
			if *retryDnsForever {
				log.Printf("Failed to resolve: %s, waiting %d seconds.", err, *delaySec)
				time.Sleep(time.Second * time.Duration(*delaySec))
			} else {
				log.Fatalf("Failed to resolve servers: %v", err)
			}
		}
	}
	for i := range servers {
		fmt.Println(servers[i], addrs[i])
	}
	ticker := time.Tick(time.Second * time.Duration(*delaySec))
	up := true
	var last *Result = nil
	for {
		now := Attempt(addrs)
		numVisible.Set(int64(now.NumOK))
		if now.Bad() != up {
			if now.Bad() {
				log.Printf("DOWN %d good %d bad", now.NumOK, now.NumBad)
				if last != nil {
					outageSec := now.Local.Sub(last.Local) / time.Second
					log.Printf("Up for %d seconds from %v to %v", outageSec, last.Local, now.Local)
				}
			} else {
				log.Printf("UP %d good %d bad", now.NumOK, now.NumBad)
				if last != nil {
					outageSec := now.Local.Sub(last.Local) / time.Second
					log.Printf("Outage lasted %d seconds from %v to %v", outageSec, last.Local, now.Local)
				}
			}
			last = &now
		} else {
			if *printNoChange {
				log.Printf("NOP %d good %d bad", now.NumOK, now.NumBad)
			}
		}
		up = now.Bad()
		<-ticker
	}
}
