package syslog

import (
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/metric"
	"github.com/influxdata/telegraf/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyslogMapperWithDefaults(t *testing.T) {
	// Init plugin
	s := newSyslog()

	// Init metrics
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC),
	)
	hostname, err := os.Hostname()
	assert.NoError(t, err)
	syslogMessage, err := s.mapMetricToSyslogMessage(m1)
	require.NoError(t, err)
	str, _ := syslogMessage.String()
	assert.Equal(t, "<0>1 2010-11-10T23:00:00Z "+hostname+" Telegraf - testmetric -", str, "Wrong syslog message")
}

func TestSyslogMapperWithDefaultSdid(t *testing.T) {
	// Init plugin
	s := newSyslog()
	s.DefaultSdid = "default@32473"

	// Init metrics
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{
			"PRI":                  uint64(0),
			"MSG":                  "Test message",
			"HOSTNAME":             "testhost",
			"APP-NAME":             "testapp",
			"PROCID":               uint64(25),
			"MSGID":                int64(555),
			"value1":               int64(2),
			"default@32473_value2": "foo",
			"value3":               float64(1.2),
		},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC),
	)

	syslogMessage, err := s.mapMetricToSyslogMessage(m1)
	require.NoError(t, err)
	str, _ := syslogMessage.String()
	assert.Equal(t, "<0>1 2010-11-10T23:00:00Z testhost testapp 25 555 [default@32473 value1=\"2\" value2=\"foo\" value3=\"1.2\"] Test message", str, "Wrong syslog message")
}

func TestSyslogMapperWithDefaultSdidAndOtherSdids(t *testing.T) {
	// Init plugin
	s := newSyslog()
	s.DefaultSdid = "default@32473"
	s.Sdids = []string{"bar@123", "foo@456"}

	// Init metrics
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{
			"PRI":                  uint64(0),
			"MSG":                  "Test message",
			"HOSTNAME":             "testhost",
			"APP-NAME":             "testapp",
			"PROCID":               uint64(25),
			"MSGID":                int64(555),
			"value1":               int64(2),
			"default@32473_value2": "default",
			"bar@123_value3":       int64(2),
			"foo@456_value4":       "foo",
		},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC),
	)

	syslogMessage, err := s.mapMetricToSyslogMessage(m1)
	require.NoError(t, err)
	str, _ := syslogMessage.String()
	assert.Equal(t, "<0>1 2010-11-10T23:00:00Z testhost testapp 25 555 [bar@123 value3=\"2\"][default@32473 value1=\"2\" value2=\"default\"][foo@456 value4=\"foo\"] Test message", str, "Wrong syslog message")
}

func TestSyslogMapperWithNoSdids(t *testing.T) {
	// Init plugin
	s := newSyslog()

	// Init metrics
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{
			"PRI":                  uint64(0),
			"MSG":                  "Test message",
			"HOSTNAME":             "testhost",
			"APP-NAME":             "testapp",
			"PROCID":               uint64(25),
			"MSGID":                int64(555),
			"value1":               int64(2),
			"default@32473_value2": "default",
			"bar@123_value3":       int64(2),
			"foo@456_value4":       "foo",
		},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC),
	)

	syslogMessage, err := s.mapMetricToSyslogMessage(m1)
	require.NoError(t, err)
	str, _ := syslogMessage.String()
	assert.Equal(t, "<0>1 2010-11-10T23:00:00Z testhost testapp 25 555 - Test message", str, "Wrong syslog message")
}

func TestGetSyslogMessageWithFramingOctectCounting(t *testing.T) {
	// Init plugin
	s := newSyslog()

	// Init metrics
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{
			"PRI":    uint64(0),
			"SOURCE": "testhost",
		},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC),
	)

	syslogMessage, err := s.mapMetricToSyslogMessage(m1)
	require.NoError(t, err)
	messageBytesWithFraming := s.getSyslogMessageBytesWithFraming(syslogMessage)

	assert.Equal(t, "58 <0>1 2010-11-10T23:00:00Z testhost Telegraf - testmetric -", string(messageBytesWithFraming), "Incorrect Octect counting framing")
}

func TestGetSyslogMessageWithFramingNonTransparent(t *testing.T) {
	// Init plugin
	s := newSyslog()
	s.Framing = NonTransparent

	// Init metrics
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{
			"PRI":    uint64(0),
			"SOURCE": "testhost",
		},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC),
	)

	syslogMessage, err := s.mapMetricToSyslogMessage(m1)
	require.NoError(t, err)
	messageBytesWithFraming := s.getSyslogMessageBytesWithFraming(syslogMessage)

	assert.Equal(t, "<0>1 2010-11-10T23:00:00Z testhost Telegraf - testmetric -\x00", string(messageBytesWithFraming), "Incorrect Octect counting framing")
}

func TestSyslogWriteWithTcp(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := newSyslog()
	s.Address = "tcp://" + listener.Addr().String()

	err = s.Connect()
	require.NoError(t, err)

	lconn, err := listener.Accept()
	require.NoError(t, err)

	testSyslogWriteWithStream(t, s, lconn)
}

func TestSyslogWriteWithUdp(t *testing.T) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	s := newSyslog()
	s.Address = "udp://" + listener.LocalAddr().String()

	err = s.Connect()
	require.NoError(t, err)

	testSyslogWriteWithPacket(t, s, listener)
}

func testSyslogWriteWithStream(t *testing.T, s *Syslog, lconn net.Conn) {
	metrics := []telegraf.Metric{}
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC))

	metrics = append(metrics, m1)
	syslogMessage, err := s.mapMetricToSyslogMessage(metrics[0])
	require.NoError(t, err)
	messageBytesWithFraming := s.getSyslogMessageBytesWithFraming(syslogMessage)

	err = s.Write(metrics)
	require.NoError(t, err)

	buf := make([]byte, 256)
	n, err := lconn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, string(messageBytesWithFraming), string(buf[:n]))
}

func testSyslogWriteWithPacket(t *testing.T, s *Syslog, lconn net.PacketConn) {
	s.Framing = NonTransparent
	metrics := []telegraf.Metric{}
	m1, _ := metric.New(
		"testmetric",
		map[string]string{},
		map[string]interface{}{},
		time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC))

	metrics = append(metrics, m1)
	syslogMessage, err := s.mapMetricToSyslogMessage(metrics[0])
	require.NoError(t, err)
	messageBytesWithFraming := s.getSyslogMessageBytesWithFraming(syslogMessage)

	err = s.Write(metrics)
	require.NoError(t, err)

	buf := make([]byte, 256)
	n, _, err := lconn.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, string(messageBytesWithFraming), string(buf[:n]))
}

func TestSyslogWriteErr(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := newSyslog()
	s.Address = "tcp://" + listener.Addr().String()

	err = s.Connect()
	require.NoError(t, err)
	s.Conn.(*net.TCPConn).SetReadBuffer(256)

	lconn, err := listener.Accept()
	require.NoError(t, err)
	lconn.(*net.TCPConn).SetWriteBuffer(256)

	metrics := []telegraf.Metric{testutil.TestMetric(1, "testerr")}

	// close the socket to generate an error
	lconn.Close()
	s.Conn.Close()
	err = s.Write(metrics)
	require.Error(t, err)
	assert.Nil(t, s.Conn)
}

func TestSyslogWriteReconnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := newSyslog()
	s.Address = "tcp://" + listener.Addr().String()

	err = s.Connect()
	require.NoError(t, err)
	s.Conn.(*net.TCPConn).SetReadBuffer(256)

	lconn, err := listener.Accept()
	require.NoError(t, err)
	lconn.(*net.TCPConn).SetWriteBuffer(256)
	lconn.Close()
	s.Conn = nil

	wg := sync.WaitGroup{}
	wg.Add(1)
	var lerr error
	go func() {
		lconn, lerr = listener.Accept()
		wg.Done()
	}()

	metrics := []telegraf.Metric{testutil.TestMetric(1, "testerr")}
	err = s.Write(metrics)
	require.NoError(t, err)

	wg.Wait()
	assert.NoError(t, lerr)

	syslogMessage, err := s.mapMetricToSyslogMessage(metrics[0])
	require.NoError(t, err)
	messageBytesWithFraming := s.getSyslogMessageBytesWithFraming(syslogMessage)
	buf := make([]byte, 256)
	n, err := lconn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, string(messageBytesWithFraming), string(buf[:n]))
}