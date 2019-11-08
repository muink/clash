// +build linux android

package dev

import (
	"errors"
	"net/url"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/google/netstack/tcpip/link/fdbased"
	stacktun "github.com/google/netstack/tcpip/link/tun"
	"github.com/google/netstack/tcpip/stack"
)

type tun struct {
	name      string
	fd        int
	closeFd   bool
	linkCache *stack.LinkEndpoint
}

func OpenTunDevice(deviceURL url.URL) (TunDevice, error) {
	switch deviceURL.Scheme {
	case "dev":
		return openDeviceByName(deviceURL.Host)
	case "fd":
		fd, err := strconv.ParseInt(deviceURL.Host, 10, 32)
		if err != nil {
			return nil, err
		}
		return openDeviceByFd(int(fd))
	}

	return nil, errors.New("Unsupported device type " + deviceURL.Scheme)
}

func (t tun) Name() string {
	return t.name
}

func (t tun) AsLinkEndpoint() (result stack.LinkEndpoint, err error) {
	if t.linkCache != nil {
		return *t.linkCache, nil
	}

	mtu, err := t.getInterfaceMtu()

	if err != nil {
		return nil, errors.New("Unable to get device mtu")
	}

	result, err = fdbased.New(&fdbased.Options{
		FDs:            []int{t.fd},
		MTU:            mtu,
		EthernetHeader: false,
	})

	t.linkCache = &result

	return result, nil
}

func (t tun) Close() {
	if t.closeFd {
		syscall.Close(t.fd)
	}
}

func openDeviceByName(name string) (TunDevice, error) {
	fd, err := stacktun.Open(name)
	if err != nil {
		return nil, err
	}

	return &tun{
		name:    name,
		fd:      fd,
		closeFd: true,
	}, nil
}

func openDeviceByFd(fd int) (TunDevice, error) {
	var ifr struct {
		name  [16]byte
		flags uint16
		_     [22]byte
	}

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TUNGETIFF, uintptr(unsafe.Pointer(&ifr)))
	if errno != 0 {
		return nil, errno
	}

	if ifr.flags&syscall.IFF_TUN == 0 || ifr.flags&syscall.IFF_NO_PI == 0 {
		return nil, errors.New("Only tun device and no pi mode supported")
	}

	return &tun{
		name:    convertInterfaceName(ifr.name),
		fd:      fd,
		closeFd: false,
	}, nil
}

func (t tun) getInterfaceMtu() (uint32, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return 0, err
	}

	defer syscall.Close(fd)

	var ifreq struct {
		name [16]byte
		mtu  int32
		_    [20]byte
	}

	copy(ifreq.name[:], t.name)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCGIFMTU, uintptr(unsafe.Pointer(&ifreq)))
	if errno != 0 {
		return 0, errno
	}

	return uint32(ifreq.mtu), nil
}

func convertInterfaceName(buf [16]byte) string {
	var n int

	for i, c := range buf {
		if c == 0 {
			n = i
			break
		}
	}

	return string(buf[:n])
}
