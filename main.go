package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

//           Bit Rate=65 Mb/s   Tx-Power=20 dBm   
//           Link Quality=51/70  Signal level=-59 dBm  
var (
	rgxpBitRate     = regexp.MustCompile(`Bit Rate=([0-9.]+) Mb/s`)
	rgxpLinkQuality = regexp.MustCompile(`Link Quality=([0-9.]+)/([0-9.]+)`)
)

func main() {
	var iface, statsdAddress string
	var sleep time.Duration

	flag.StringVar(&iface, "iface", "wlan0", "wireless network interface")
	flag.StringVar(&statsdAddress, "statsd", "127.0.0.1:8125", "udp statsd address")
	flag.DurationVar(&sleep, "sleep", 10*time.Second, "sleep duration between checks")
	flag.Parse()

	conn, err := open(statsdAddress)
	if err != nil {
		log.Fatalln(err)
	}

	for {
		if bitRate, linkQuality, err := check(iface); err != nil {
			log.Printf("FAILED %v\n", err)
			conn.Printf("wificheck.run.failed:1|g\n")
		} else {
			log.Printf("OK bit rate: %.3f, quality: %.3f\n", bitRate, linkQuality)
			conn.Printf("wificheck.run.ok:1|g\n")
			conn.Printf("wificheck.bit_rate:%.3f|g\n", bitRate)
			conn.Printf("wificheck.quality:%.3f|g\n", linkQuality)
		}

		time.Sleep(sleep)
	}
}

func check(iface string) (float64, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "iwconfig", iface)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("Couldn't run command, %v", err)
	}

	var bitRate, linkQuality float64

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		ln := scanner.Text()
		if m := rgxpBitRate.FindStringSubmatch(ln); len(m) > 0 {
			n, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return 0, 0, fmt.Errorf("Couldn't parse bit rate, %v", err)
			}

			bitRate = n
		}

		if m := rgxpLinkQuality.FindStringSubmatch(ln); len(m) > 0 {
			n1, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return 0, 0, fmt.Errorf("Couldn't parse link quality, %v", err)
			}

			n2, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				return 0, 0, fmt.Errorf("Couldn't parse link quality, %v", err)
			}

			if n2 > 0 {
				linkQuality = n1 / n2
			}
		}
	}

	return bitRate, linkQuality, nil
}

type connection struct {
	addr *net.UDPAddr
	conn *net.UDPConn
}

func open(writeAddress string) (*connection, error) {
	addr, err := net.ResolveUDPAddr("udp", writeAddress)
	if err != nil {
		return nil, fmt.Errorf("Couldn't resolve %q, %v", writeAddress, err)
	}

	bind, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return nil, fmt.Errorf("Couldn't resolve :0, %v", err)
	}

	conn, err := net.ListenUDP("udp", bind)
	if err != nil {
		return nil, fmt.Errorf("Couldn't listen %s, %v", addr, err)
	}

	return &connection{
		addr: addr,
		conn: conn,
	}, nil
}

func (c *connection) Printf(tpl string, v ...interface{}) {
	c.conn.WriteToUDP([]byte(fmt.Sprintf(tpl, v...)), c.addr)
}
