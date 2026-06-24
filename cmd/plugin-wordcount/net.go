package main

import "net"

func netDial(network string, address string) (net.Conn, error) {
	return net.Dial(network, address)
}
