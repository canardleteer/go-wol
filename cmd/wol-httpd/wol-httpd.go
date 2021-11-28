package main

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sabhiram/go-wol/wol"
	flag "github.com/spf13/pflag"
	"net"
	"net/http"
	"os"
)

// This is truly a hack, sidestepping some of the go-wol niceitys, but
// providing an HTTP interface for the use case and time budget I have.

var (
	listenPort         uint
	listenAddr         string
	broadcastInterface string
	broadcastIP        string
	udpPort            uint
	targetMAC          string
)

func main() {
	// Setup our logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	flag.UintVar(&listenPort, "listen-port", 8080, "http port to listen to")
	flag.StringVar(&listenAddr, "listen-addr", "0.0.0.0", "http interface to bind to")
	flag.StringVar(&broadcastInterface, "broadcast-interface", "", "WoL broadcast interface")
	flag.StringVar(&broadcastIP, "broadcast-ip", "255.255.255.255", "WoL broadcast ip")
	flag.UintVar(&udpPort, "broadcast-port", 9, "WoL broadcast UDP port")
	flag.StringVar(&targetMAC, "target-mac", "e0:da:dc:06:48:8f", "WoL target mac")
	flag.Parse()

	// Setup an http listener from here.
	http.HandleFunc("/wolGenerate", wolHandler)

	listenCombo := fmt.Sprintf("%s:%d", listenAddr, listenPort)

	log.Info().Msg(fmt.Sprintf("Starting server at %s\n", listenCombo))
	if err := http.ListenAndServe(listenCombo, nil); err != nil {
		log.Fatal().Err(err)
	}
}

func wolHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/wolGenerate" {
		http.Error(w, "404 not found.", http.StatusNotFound)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method is not supported.", http.StatusNotFound)
		return
	}

	// Do the thing
	err := doTheThing()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}

	fmt.Fprintf(w, "Successfully injected the WoL packet on the network for %s.", targetMAC)
}

// doTheThing basically just wraps all the WoL packet gen stuff. Much of this
// code is grifted from wol.go
func doTheThing() error {
	var err error
	bcastInterface := broadcastInterface
	macAddr := targetMAC

	// Populate the local address in the event that the broadcast interface has
	// been set.
	var localAddr *net.UDPAddr
	if bcastInterface != "" {
		localAddr, err = ipFromInterface(bcastInterface)
		if err != nil {
			return err
		}
	}

	// The address to broadcast to is usually the default `255.255.255.255` but
	// can be overloaded by specifying an override in the CLI arguments.
	bcastAddr := fmt.Sprintf("%s:%d", broadcastIP, udpPort)
	udpAddr, err := net.ResolveUDPAddr("udp", bcastAddr)
	if err != nil {
		return err
	}

	// Build the magic packet.
	mp, err := wol.New(macAddr)
	if err != nil {
		return err
	}

	// Grab a stream of bytes to send.
	bs, err := mp.Marshal()
	if err != nil {
		return err
	}

	// Grab a UDP connection to send our packet of bytes.
	conn, err := net.DialUDP("udp", localAddr, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Info().Msg(fmt.Sprintf("Attempting to send a magic packet to MAC %s\n", macAddr))
	log.Info().Msg(fmt.Sprintf("... Broadcasting to: %s\n", bcastAddr))

	n, err := conn.Write(bs)
	if err == nil && n != 102 {
		err = fmt.Errorf("magic packet sent was %d bytes (expected 102 bytes sent)", n)
	}
	if err != nil {
		return err
	}

	log.Info().Msg(fmt.Sprintf("Magic packet sent successfully to %s\n", macAddr))
	return nil
}

// ipFromInterface returns a `*net.UDPAddr` from a network interface name.
func ipFromInterface(iface string) (*net.UDPAddr, error) {
	ief, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}

	addrs, err := ief.Addrs()
	if err == nil && len(addrs) <= 0 {
		err = fmt.Errorf("no address associated with interface %s", iface)
	}
	if err != nil {
		return nil, err
	}

	// Validate that one of the addrs is a valid network IP address.
	for _, addr := range addrs {
		switch ip := addr.(type) {
		case *net.IPNet:
			if !ip.IP.IsLoopback() && ip.IP.To4() != nil {
				return &net.UDPAddr{
					IP: ip.IP,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("no address associated with interface %s", iface)
}
