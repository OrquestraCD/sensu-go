package backend

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"testing"
	"time"

	nsq "github.com/nsqio/go-nsq"
	"github.com/sensu/sensu-go/transport"
	"github.com/sensu/sensu-go/types"
	"github.com/stretchr/testify/assert"
)

func testWithTempDir(f func(string)) {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "sensu")
	defer os.RemoveAll(tmpDir)
	if err != nil {
		log.Fatal(err.Error())
	}
	f(tmpDir)
}

func TestHTTPListener(t *testing.T) {
	testWithTempDir(func(path string) {
		// This will likely bite us in the butt eventually, but for now, we need a
		// way to get random available ports for etcd so that we can still run
		// tests in parallel since we might have multiple etcds running.
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			log.Panic(err)
		}
		addr, err := net.ResolveTCPAddr("tcp", l.Addr().String())
		if err != nil {
			log.Panic(err)
		}
		clURL := fmt.Sprintf("http://127.0.0.1:%d", addr.Port)
		apURL := fmt.Sprintf("http://127.0.0.1:%d", addr.Port+1)
		initCluster := fmt.Sprintf("default=%s", apURL)
		fmt.Println(initCluster)
		l.Close()

		b, err := NewBackend(&Config{
			Port:                31337,
			StateDir:            path,
			EtcdClientListenURL: clURL,
			EtcdPeerListenURL:   apURL,
			EtcdInitialCluster:  initCluster,
		})
		assert.NoError(t, err)
		if err != nil {
			assert.FailNow(t, "failed to start backend")
		}

		err = b.Run()
		assert.NoError(t, err)

		for i := 0; i < 5; i++ {
			conn, derr := net.Dial("tcp", "localhost:31337")
			if derr != nil {
				fmt.Println("Waiting for backend to start")
				time.Sleep(time.Duration(i) * time.Second)
			} else {
				conn.Close()
				continue
			}
		}

		client, err := transport.Connect("ws://localhost:31337")
		assert.NoError(t, err)
		assert.NotNil(t, client)

		err = client.Send(types.AgentHandshakeType, []byte("{}"))
		assert.NoError(t, err)
		mt, _, err := client.Receive()
		assert.NoError(t, err)
		assert.Equal(t, types.BackendHandshakeType, mt)

		assert.NoError(t, client.Close())

		// Test NSQD
		nsqCfg := nsq.NewConfig()
		producer, err := nsq.NewProducer("127.0.0.1:4150", nsqCfg)
		assert.NoError(t, err)

		err = producer.Publish("test_topic", []byte("{}"))
		assert.NoError(t, err)
		b.Stop()
	})
}