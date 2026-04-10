//go:build windows

package main

import (
	"fmt"
	"log"
	"net"

	"github.com/grandcat/zeroconf"
)

// serviceType is het mDNS-servicetype dat de client adverteert.
// Home Assistant zoekt naar dit type voor automatische ontdekking.
const serviceType = "_ha-shutdown._tcp"

// startMDNS adverteert de client via mDNS (Zeroconf) zodat Home Assistant
// hem automatisch kan vinden op het lokale netwerk.
//
// De interface die gebruikt wordt voor internet-toegang wordt bepaald via
// de routeringstabel van het OS (zie internetInterface). Dit zorgt er voor
// dat bij systemen met meerdere adapters (VPN, virtualisatie, WSL, …) altijd
// het juiste IP-adres wordt geadverteerd.
func startMDNS(port int) (*zeroconf.Server, error) {
	hostname := mustHostname()

	iface, ip, err := internetInterface()
	if err != nil {
		// Fallback: laat zeroconf zelf kiezen (kan fout gaan bij meerdere adapters)
		log.Printf("mDNS: kan internet-interface niet bepalen (%v), gebruik fallback", err)
		return zeroconfRegister(hostname, port, nil)
	}

	log.Printf("mDNS: adverteer op interface %q (%s)", iface.Name, ip)
	return zeroconfRegister(hostname, port, []net.Interface{*iface})
}

// zeroconfRegister registreert de mDNS-service op de opgegeven interfaces.
func zeroconfRegister(hostname string, port int, ifaces []net.Interface) (*zeroconf.Server, error) {
	srv, err := zeroconf.Register(
		hostname,
		serviceType,
		"local.",
		port,
		[]string{
			fmt.Sprintf("hostname=%s", hostname),
			fmt.Sprintf("version=%s", version),
		},
		ifaces, // nil = alle niet-loopback interfaces
	)
	if err != nil {
		return nil, fmt.Errorf("mDNS-registratie mislukt: %w", err)
	}
	return srv, nil
}

// internetInterface bepaalt welke netwerkinterface en welk lokaal IP-adres
// het OS gebruikt voor internet-verkeer.
//
// Techniek: open een UDP-socket naar 8.8.8.8:80. Omdat UDP verbindingsloos
// is, verstuurt het OS geen enkel pakket, maar kiest het WEL de juiste
// uitgaande interface op basis van de routeringstabel. Het lokale adres van
// de socket is dan precies het adres dat we willen adverteren.
//
// Dit werkt correct op systemen met:
//   - VPN-adapters (split-tunnel of full-tunnel)
//   - VMware / Hyper-V / VirtualBox virtuele adapters
//   - WSL-netwerk-adapters
//   - Meerdere fysieke netwerkkaarten
func internetInterface() (*net.Interface, *net.IP, error) {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return nil, nil, fmt.Errorf("UDP-dial mislukt: %w", err)
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddr == nil {
		return nil, nil, fmt.Errorf("kan lokaal adres niet bepalen")
	}
	targetIP := localAddr.IP

	// Zoek de interface die dit IP-adres heeft
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("interfaces ophalen mislukt: %w", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.Equal(targetIP) {
				iface := iface // kopieer lokaal: pointer naar loop-variabele vermijden
				return &iface, &targetIP, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("geen interface gevonden voor IP %s", targetIP)
}
