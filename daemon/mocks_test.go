package daemon

import (
	"io/fs"
	"net/netip"
	"testing"

	"github.com/NordSecurity/nordvpn-linux/config"
	"github.com/NordSecurity/nordvpn-linux/core/mesh"
	"github.com/NordSecurity/nordvpn-linux/daemon/vpn"
	"github.com/NordSecurity/nordvpn-linux/events"
	"github.com/NordSecurity/nordvpn-linux/networker"
	"github.com/google/uuid"
)

type mockNetworker struct {
	dns               []string
	allowlist         config.Allowlist
	vpnActive         bool
	lanDiscovery      bool
	meshPeers         mesh.MachinePeers
	setDNSErr         error
	setWhitelistErr   error
	unsetWhitelistErr error
}

func (mockNetworker) Start(
	vpn.Credentials,
	vpn.ServerData,
	config.Allowlist,
	config.DNS,
) error {
	return nil
}
func (*mockNetworker) Stop() error      { return nil }
func (*mockNetworker) UnSetMesh() error { return nil }

func (mn *mockNetworker) SetDNS(nameservers []string) error {
	mn.dns = nameservers
	return mn.setDNSErr
}

func (*mockNetworker) UnsetDNS() error { return nil }

func (mn *mockNetworker) IsVPNActive() bool {
	return mn.vpnActive
}

func (*mockNetworker) ConnectionStatus() (networker.ConnectionStatus, error) {
	return networker.ConnectionStatus{}, nil
}

func (*mockNetworker) EnableFirewall() error  { return nil }
func (*mockNetworker) DisableFirewall() error { return nil }
func (*mockNetworker) EnableRouting()         {}
func (*mockNetworker) DisableRouting()        {}

func (mn *mockNetworker) SetAllowlist(allowlist config.Allowlist) error {
	if mn.setWhitelistErr != nil {
		return mn.setWhitelistErr
	}

	mn.allowlist = allowlist
	return nil
}

func (mn *mockNetworker) UnsetAllowlist() error {
	if mn.unsetWhitelistErr != nil {
		return mn.unsetWhitelistErr
	}

	mn.allowlist.Ports.TCP = make(config.PortSet)
	mn.allowlist.Ports.UDP = make(config.PortSet)
	mn.allowlist.Subnets = make(config.Subnets)
	return nil
}

func (*mockNetworker) IsNetworkSet() bool                   { return false }
func (*mockNetworker) SetKillSwitch(config.Allowlist) error { return nil }
func (*mockNetworker) UnsetKillSwitch() error               { return nil }
func (*mockNetworker) PermitIPv6() error                    { return nil }
func (*mockNetworker) DenyIPv6() error                      { return nil }
func (*mockNetworker) SetVPN(vpn.VPN)                       {}
func (*mockNetworker) LastServerName() string               { return "" }

func (mn *mockNetworker) SetLanDiscoveryAndResetMesh(enabled bool, peers mesh.MachinePeers) {
	mn.meshPeers = peers
	mn.lanDiscovery = enabled
}

func (mn *mockNetworker) SetLanDiscovery(enabled bool) {
	mn.lanDiscovery = enabled
}

var tplNameserversV4 config.DNS = []string{
	"103.86.96.96",
	"103.86.99.99",
}

var tplNameserversV6 config.DNS = []string{
	"2400:bb40:4444::103",
	"2400:bb40:8888::103",
}

var defaultNameserversV4 config.DNS = []string{
	"103.86.96.100",
	"103.86.99.100",
}

var defaultNameserversV6 config.DNS = []string{
	"2400:bb40:4444::100",
	"2400:bb40:8888::100",
}

type mockDNSGetter struct {
}

func (md *mockDNSGetter) Get(isThreatProtectionLite bool, isIPv6 bool) []string {
	if isThreatProtectionLite {
		nameservers := tplNameserversV4
		if isIPv6 {
			nameservers = append(nameservers, tplNameserversV6...)
		}
		return nameservers
	}

	nameservers := defaultNameserversV4
	if isIPv6 {
		nameservers = append(nameservers, defaultNameserversV6...)
	}
	return nameservers
}

type mockPublisherSubcriber struct {
	eventPublished bool
}

func (mp *mockPublisherSubcriber) Publish(message bool) {
	mp.eventPublished = true
}
func (*mockPublisherSubcriber) Subscribe(handler events.Handler[bool]) {}

type filesystemMock struct {
	files    map[string][]byte
	WriteErr error
}

func (fm *filesystemMock) FileExists(location string) bool {
	_, ok := fm.files[location]

	return ok
}

func (fm *filesystemMock) CreateFile(location string, mode fs.FileMode) error {
	fm.files[location] = []byte{}
	return nil
}

func (fm *filesystemMock) ReadFile(location string) ([]byte, error) {
	return fm.files[location], nil
}

func (fm *filesystemMock) WriteFile(location string, data []byte, mode fs.FileMode) error {
	if fm.WriteErr != nil {
		return fm.WriteErr
	}

	fm.files[location] = data
	return nil
}

func newFilesystemMock(t *testing.T) filesystemMock {
	t.Helper()

	return filesystemMock{
		files: make(map[string][]byte),
	}
}

type machineIDGetterMock struct {
	machineID uuid.UUID
}

func (mid *machineIDGetterMock) GetMachineID() uuid.UUID {
	return mid.machineID
}

type RegistryMock struct {
	peers mesh.MachinePeers
}

func (*RegistryMock) Register(token string, self mesh.Machine) (*mesh.Machine, error) {
	return nil, nil
}
func (*RegistryMock) Update(token string, id uuid.UUID, endpoints []netip.AddrPort) error { return nil }

func (*RegistryMock) Configure(
	token string,
	id uuid.UUID,
	peerID uuid.UUID,
	doIAllowInbound bool,
	doIAllowRouting bool,
	doIAllowLocalNetwork bool,
	doIAllowFileshare bool,
	alwaysAcceptFiles bool,
) error {
	return nil
}

func (*RegistryMock) Unregister(token string, self uuid.UUID) error { return nil }
func (*RegistryMock) Local(token string) (mesh.Machines, error)     { return mesh.Machines{}, nil }

func (rm *RegistryMock) List(token string, self uuid.UUID) (mesh.MachinePeers, error) {
	return rm.peers, nil
}

func (*RegistryMock) Map(token string, self uuid.UUID) (*mesh.MachineMap, error) { return nil, nil }
func (*RegistryMock) Unpair(token string, self uuid.UUID, peer uuid.UUID) error  { return nil }

func (*RegistryMock) NotifyNewTransfer(
	token string,
	self uuid.UUID,
	peer uuid.UUID,
	fileName string,
	fileCount int,
) error {
	return nil
}
