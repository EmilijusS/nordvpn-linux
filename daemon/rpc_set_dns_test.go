package daemon

import (
	"context"
	"fmt"
	"net/netip"
	"testing"

	"github.com/NordSecurity/nordvpn-linux/config"
	"github.com/NordSecurity/nordvpn-linux/daemon/pb"
	"github.com/NordSecurity/nordvpn-linux/events"
	"github.com/NordSecurity/nordvpn-linux/network"
	"github.com/NordSecurity/nordvpn-linux/test/category"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var dnsMock config.DNS = config.DNS{"0.0.0.0", "8.8.8.8", "1.1.1.1"}
var currentDNSMock config.DNS = config.DNS{"131.244.140.126", "194.182.108.28", "124.83.117.225"}
var dnsV6Mock config.DNS = config.DNS{
	"93c2:2773:ef6c:6a9b:9d92:1349:bd4e:c11d",
	"9198:0c8e:0d72:6081:9c47:b378:916c:8d5c",
	"4aac:965f:f4fc:ae1d:2213:92e6:c6aa:3fcf"}

type mockPublisherSubscriberDNS struct {
	eventPublished bool
}

func (mp *mockPublisherSubscriberDNS) Publish(message events.DataDNS) {
	mp.eventPublished = true
}
func (*mockPublisherSubscriberDNS) Subscribe(handler events.Handler[events.DataDNS]) {}

func TestSetDNS_Success(t *testing.T) {
	category.Set(t, category.Unit)

	tests := []struct {
		name                string
		requestedDNS        config.DNS
		currentDNS          config.DNS
		expectedDNS         config.DNS
		expectedDNSInConfig config.DNS
		tpl                 bool
		ipv6                bool
		expectedTPL         bool
	}{
		{
			name:                "set new DNS",
			requestedDNS:        dnsMock,
			expectedDNS:         dnsMock,
			expectedDNSInConfig: dnsMock,
		},
		{
			name:                "overwrite DNS",
			requestedDNS:        dnsMock,
			currentDNS:          currentDNSMock,
			expectedDNS:         dnsMock,
			expectedDNSInConfig: dnsMock,
		},
		{
			name:                "remove single address",
			requestedDNS:        currentDNSMock[0:1],
			currentDNS:          currentDNSMock,
			expectedDNS:         currentDNSMock[0:1],
			expectedDNSInConfig: currentDNSMock[0:1],
		},
		{
			name:                "add single address",
			requestedDNS:        currentDNSMock,
			currentDNS:          currentDNSMock[0:1],
			expectedDNS:         currentDNSMock,
			expectedDNSInConfig: currentDNSMock,
		},
		{
			name:                "set new DNS ipv6",
			requestedDNS:        dnsV6Mock,
			expectedDNS:         dnsV6Mock,
			expectedDNSInConfig: dnsV6Mock,
		},
		{
			name:                "remove custom dns ipv4",
			requestedDNS:        nil,
			currentDNS:          dnsMock,
			expectedDNS:         defaultNameserversV4,
			expectedDNSInConfig: nil,
		},
		{
			name:                "remove custom dns ipv4 tpl",
			requestedDNS:        nil,
			currentDNS:          dnsMock,
			expectedDNS:         tplNameserversV4,
			expectedDNSInConfig: nil,
			tpl:                 true,
			expectedTPL:         true,
		},
		{
			name:                "remove custom dns ipv6",
			requestedDNS:        nil,
			currentDNS:          dnsMock,
			expectedDNS:         append(defaultNameserversV4, defaultNameserversV6...),
			expectedDNSInConfig: nil,
			ipv6:                true,
		},
		{
			name:                "remove custom dns ipv6 tpl",
			requestedDNS:        nil,
			currentDNS:          dnsMock,
			expectedDNS:         append(tplNameserversV4, tplNameserversV6...),
			expectedDNSInConfig: nil,
			tpl:                 true,
			ipv6:                true,
			expectedTPL:         true,
		},
		{
			name:                "overwrite tpl ipv4",
			requestedDNS:        dnsMock,
			expectedDNS:         dnsMock,
			expectedDNSInConfig: dnsMock,
			tpl:                 true,
			expectedTPL:         false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ipv4Endpoint := netip.MustParseAddr("142.114.71.151")
			ipv6Endpoint := []netip.Addr{
				netip.MustParseAddr("cfa2:f822:e7fb:20a6:d4c0:b9fd:2901:95f0"),
			}

			uuid, _ := uuid.NewUUID()
			filesystem := newFilesystemMock(t)
			configManager := config.NewFilesystemConfigManager(
				"/location", "/vault", "",
				&machineIDGetterMock{machineID: uuid},
				&filesystem)

			configManager.SaveWith(func(c config.Config) config.Config {
				c.AutoConnectData = config.AutoConnectData{
					DNS:                  test.currentDNS,
					ThreatProtectionLite: test.tpl,
				}
				c.IPv6 = test.ipv6

				return c
			})

			networker := mockNetworker{}
			publisher := mockPublisherSubscriberDNS{}
			dnsGetter := mockDNSGetter{}

			var endpoint network.Endpoint
			if test.ipv6 {
				endpoint = network.NewIPv6Endpoint(ipv6Endpoint)
			} else {
				endpoint = network.NewIPv4Endpoint(ipv4Endpoint)
			}

			rpc := RPC{
				cm:          configManager,
				netw:        &networker,
				nameservers: &dnsGetter,
				events:      &Events{Settings: &SettingsEvents{DNS: &publisher}},
				endpoint:    endpoint,
			}

			resp, err := rpc.SetDNS(context.Background(),
				&pb.SetDNSRequest{Dns: test.requestedDNS})

			assert.Nil(t, err, "RPC failed")
			assert.IsType(t, &pb.SetDNSResponse{Response: &pb.SetDNSResponse_SetDnsStatus{}}, resp,
				"Non-empty response received, empty response indicates success")

			assert.Equal(t, test.expectedDNS, config.DNS(networker.dns), "Invalid DNS was configured.")

			var cfg config.Config
			configManager.Load(&cfg)
			assert.Equal(t, test.expectedDNSInConfig, cfg.AutoConnectData.DNS,
				"Invalid DNS was saved in the configuration.")
			assert.Equal(t, test.expectedTPL, cfg.AutoConnectData.ThreatProtectionLite,
				"Threat protection lite was not properly configured after enabling DNS.")
		})
	}
}

func TestSetDNS_Errors(t *testing.T) {
	category.Set(t, category.Unit)

	tests := []struct {
		name             string
		requestedDNS     config.DNS
		currentDNS       config.DNS
		setDNSErr        error
		writeConfigErr   error
		expectedResponse *pb.SetDNSResponse
	}{
		{
			name:         "too many nameservers",
			requestedDNS: config.DNS{"0.0.0.0", "8.8.8.8", "1.1.1.1", "1.2.3.4"},
			expectedResponse: &pb.SetDNSResponse{
				Response: &pb.SetDNSResponse_SetDnsStatus{SetDnsStatus: pb.SetDNSStatus_TOO_MANY_VALUES},
			},
		},
		{
			name:         "already set",
			requestedDNS: dnsMock,
			currentDNS:   dnsMock,
			expectedResponse: &pb.SetDNSResponse{
				Response: &pb.SetDNSResponse_ErrorCode{ErrorCode: pb.SetErrorCode_ALREADY_SET},
			},
		},
		{
			name:         "invalid address",
			requestedDNS: config.DNS{"aaasd"},
			expectedResponse: &pb.SetDNSResponse{
				Response: &pb.SetDNSResponse_SetDnsStatus{SetDnsStatus: pb.SetDNSStatus_INVALID_DNS_ADDRESS},
			},
		},
		{
			name:         "network error",
			requestedDNS: dnsMock,
			setDNSErr:    fmt.Errorf("failed to set dns"),
			expectedResponse: &pb.SetDNSResponse{
				Response: &pb.SetDNSResponse_ErrorCode{ErrorCode: pb.SetErrorCode_FAILURE},
			},
		},
		{
			name:           "config error",
			requestedDNS:   dnsMock,
			writeConfigErr: fmt.Errorf("failed to save config"),
			expectedResponse: &pb.SetDNSResponse{
				Response: &pb.SetDNSResponse_ErrorCode{ErrorCode: pb.SetErrorCode_CONFIG_ERROR},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uuid, _ := uuid.NewUUID()
			filesystem := newFilesystemMock(t)
			filesystem.WriteErr = test.writeConfigErr
			configManager := config.NewFilesystemConfigManager(
				"/location", "/vault", "",
				&machineIDGetterMock{machineID: uuid},
				&filesystem)

			configManager.SaveWith(func(c config.Config) config.Config {
				c.AutoConnectData = config.AutoConnectData{
					DNS: test.currentDNS,
				}

				return c
			})

			networker := mockNetworker{setDNSErr: test.setDNSErr}
			publisher := mockPublisherSubscriberDNS{}
			dnsGetter := mockDNSGetter{}

			rpc := RPC{
				cm:          configManager,
				netw:        &networker,
				nameservers: &dnsGetter,
				events:      &Events{Settings: &SettingsEvents{DNS: &publisher}},
			}

			resp, err := rpc.SetDNS(context.Background(),
				&pb.SetDNSRequest{Dns: test.requestedDNS})

			assert.Nil(t, err, "RPC failed")
			assert.Equal(t, test.expectedResponse, resp)
		})
	}
}
