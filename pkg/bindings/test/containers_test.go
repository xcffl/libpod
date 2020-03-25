package test_bindings

import (
	"github.com/containers/libpod/libpod/define"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/containers/libpod/pkg/bindings"
	"github.com/containers/libpod/pkg/bindings/containers"
	"github.com/containers/libpod/pkg/specgen"
	"github.com/containers/libpod/test/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Podman containers ", func() {
	var (
		bt  *bindingTest
		s   *gexec.Session
		err error
	)

	BeforeEach(func() {
		bt = newBindingTest()
		bt.RestoreImagesFromCache()
		s = bt.startAPIService()
		time.Sleep(1 * time.Second)
		err := bt.NewConnection()
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		s.Kill()
		//bt.cleanup()
	})

	It("podman pause a bogus container", func() {
		// Pausing bogus container should return 404
		err = containers.Pause(bt.conn, "foobar")
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusNotFound))
	})

	It("podman unpause a bogus container", func() {
		// Unpausing bogus container should return 404
		err = containers.Unpause(bt.conn, "foobar")
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusNotFound))
	})

	It("podman pause a running container by name", func() {
		// Pausing by name should work
		var name = "top"
		_, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, name)
		Expect(err).To(BeNil())

		// Ensure container is paused
		data, err := containers.Inspect(bt.conn, name, nil)
		Expect(err).To(BeNil())
		Expect(data.State.Status).To(Equal("paused"))
	})

	It("podman pause a running container by id", func() {
		// Pausing by id should work
		var name = "top"
		cid, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, cid)
		Expect(err).To(BeNil())

		// Ensure container is paused
		data, err := containers.Inspect(bt.conn, cid, nil)
		Expect(err).To(BeNil())
		Expect(data.State.Status).To(Equal("paused"))
	})

	It("podman unpause a running container by name", func() {
		// Unpausing by name should work
		var name = "top"
		_, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, name)
		Expect(err).To(BeNil())
		err = containers.Unpause(bt.conn, name)
		Expect(err).To(BeNil())

		// Ensure container is unpaused
		data, err := containers.Inspect(bt.conn, name, nil)
		Expect(err).To(BeNil())
		Expect(data.State.Status).To(Equal("running"))
	})

	It("podman unpause a running container by ID", func() {
		// Unpausing by ID should work
		var name = "top"
		_, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		// Pause by name
		err = containers.Pause(bt.conn, name)
		//paused := "paused"
		//_, err = containers.Wait(bt.conn, cid, &paused)
		//Expect(err).To(BeNil())
		err = containers.Unpause(bt.conn, name)
		Expect(err).To(BeNil())

		// Ensure container is unpaused
		data, err := containers.Inspect(bt.conn, name, nil)
		Expect(err).To(BeNil())
		Expect(data.State.Status).To(Equal("running"))
	})

	It("podman pause a paused container by name", func() {
		// Pausing a paused container by name should fail
		var name = "top"
		_, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, name)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, name)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusInternalServerError))
	})

	It("podman pause a paused container by id", func() {
		// Pausing a paused container by id should fail
		var name = "top"
		cid, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, cid)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, cid)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusInternalServerError))
	})

	It("podman pause a stopped container by name", func() {
		// Pausing a stopped container by name should fail
		var name = "top"
		_, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Stop(bt.conn, name, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, name)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusInternalServerError))
	})

	It("podman pause a stopped container by id", func() {
		// Pausing a stopped container by id should fail
		var name = "top"
		cid, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Stop(bt.conn, cid, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, cid)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusInternalServerError))
	})

	It("podman remove a paused container by id without force", func() {
		// Removing a paused container without force should fail
		var name = "top"
		cid, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, cid)
		Expect(err).To(BeNil())
		err = containers.Remove(bt.conn, cid, &bindings.PFalse, &bindings.PFalse)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusInternalServerError))
	})

	It("podman remove a paused container by id with force", func() {
		// FIXME: Skip on F31 and later
		host := utils.GetHostDistributionInfo()
		osVer, err := strconv.Atoi(host.Version)
		Expect(err).To(BeNil())
		if host.Distribution == "fedora" && osVer >= 31 {
			Skip("FIXME: https://github.com/containers/libpod/issues/5325")
		}

		// Removing a paused container with force should work
		var name = "top"
		cid, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, cid)
		Expect(err).To(BeNil())
		err = containers.Remove(bt.conn, cid, &bindings.PTrue, &bindings.PFalse)
		Expect(err).To(BeNil())
	})

	It("podman stop a paused container by name", func() {
		// Stopping a paused container by name should fail
		var name = "top"
		_, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, name)
		Expect(err).To(BeNil())
		err = containers.Stop(bt.conn, name, nil)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusInternalServerError))
	})

	It("podman stop a paused container by id", func() {
		// Stopping a paused container by id should fail
		var name = "top"
		cid, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Pause(bt.conn, cid)
		Expect(err).To(BeNil())
		err = containers.Stop(bt.conn, cid, nil)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusInternalServerError))
	})

	It("podman stop a running container by name", func() {
		// Stopping a running container by name should work
		var name = "top"
		_, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Stop(bt.conn, name, nil)
		Expect(err).To(BeNil())

		// Ensure container is stopped
		data, err := containers.Inspect(bt.conn, name, nil)
		Expect(err).To(BeNil())
		Expect(isStopped(data.State.Status)).To(BeTrue())
	})

	It("podman stop a running container by ID", func() {
		// Stopping a running container by ID should work
		var name = "top"
		cid, err := bt.RunTopContainer(&name, &bindings.PFalse, nil)
		Expect(err).To(BeNil())
		err = containers.Stop(bt.conn, cid, nil)
		Expect(err).To(BeNil())

		// Ensure container is stopped
		data, err := containers.Inspect(bt.conn, name, nil)
		Expect(err).To(BeNil())
		Expect(isStopped(data.State.Status)).To(BeTrue())
	})

	It("podman wait no condition", func() {
		var (
			name           = "top"
			exitCode int32 = -1
		)
		_, err := containers.Wait(bt.conn, "foobar", nil)
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusNotFound))

		errChan := make(chan error)
		_, err = bt.RunTopContainer(&name, nil, nil)
		Expect(err).To(BeNil())
		go func() {
			exitCode, err = containers.Wait(bt.conn, name, nil)
			errChan <- err
			close(errChan)
		}()
		err = containers.Stop(bt.conn, name, nil)
		Expect(err).To(BeNil())
		wait := <-errChan
		Expect(wait).To(BeNil())
		Expect(exitCode).To(BeNumerically("==", 143))
	})

	It("podman wait to pause|unpause condition", func() {
		var (
			name           = "top"
			exitCode int32 = -1
			pause          = define.ContainerStatePaused
			running        = define.ContainerStateRunning
		)
		errChan := make(chan error)
		_, err := bt.RunTopContainer(&name, nil, nil)
		Expect(err).To(BeNil())
		go func() {
			exitCode, err = containers.Wait(bt.conn, name, &pause)
			errChan <- err
			close(errChan)
		}()
		err = containers.Pause(bt.conn, name)
		Expect(err).To(BeNil())
		wait := <-errChan
		Expect(wait).To(BeNil())
		Expect(exitCode).To(BeNumerically("==", -1))

		errChan = make(chan error)
		go func() {
			_, waitErr := containers.Wait(bt.conn, name, &running)
			errChan <- waitErr
			close(errChan)
		}()
		err = containers.Unpause(bt.conn, name)
		Expect(err).To(BeNil())
		unPausewait := <-errChan
		Expect(unPausewait).To(BeNil())
		Expect(exitCode).To(BeNumerically("==", -1))
	})

	It("run  healthcheck", func() {
		bt.runPodman([]string{"run", "-d", "--name", "hc", "--health-interval", "disable", "--health-retries", "2", "--health-cmd", "ls / || exit 1", alpine.name, "top"})

		// bogus name should result in 404
		_, err := containers.RunHealthCheck(bt.conn, "foobar")
		Expect(err).ToNot(BeNil())
		code, _ := bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusNotFound))

		// a container that has no healthcheck should be a 409
		var name = "top"
		bt.RunTopContainer(&name, &bindings.PFalse, nil)
		_, err = containers.RunHealthCheck(bt.conn, name)
		Expect(err).ToNot(BeNil())
		code, _ = bindings.CheckResponseCode(err)
		Expect(code).To(BeNumerically("==", http.StatusConflict))

		// TODO for the life of me, i cannot get this to work. maybe another set
		// of eyes will
		// successful healthcheck
		//status := "healthy"
		//for i:=0; i < 10; i++ {
		//	result, err := containers.RunHealthCheck(connText, "hc")
		//	Expect(err).To(BeNil())
		//	if result.Status != "healthy" {
		//		fmt.Println("Healthcheck container still starting, retrying in 1 second")
		//		time.Sleep(1 * time.Second)
		//		continue
		//	}
		//	status = result.Status
		//	break
		//}
		//Expect(status).To(Equal("healthy"))

		// TODO enable this when wait is working
		// healthcheck on a stopped container should be a 409
		//err = containers.Stop(connText, "hc", nil)
		//Expect(err).To(BeNil())
		//_, err = containers.Wait(connText, "hc")
		//Expect(err).To(BeNil())
		//_, err = containers.RunHealthCheck(connText, "hc")
		//code, _ = bindings.CheckResponseCode(err)
		//Expect(code).To(BeNumerically("==", http.StatusConflict))
	})

	It("logging", func() {
		stdoutChan := make(chan string, 10)
		s := specgen.NewSpecGenerator(alpine.name)
		s.Terminal = true
		s.Command = []string{"date", "-R"}
		r, err := containers.CreateWithSpec(bt.conn, s)
		Expect(err).To(BeNil())
		err = containers.Start(bt.conn, r.ID, nil)
		Expect(err).To(BeNil())

		_, err = containers.Wait(bt.conn, r.ID, nil)
		Expect(err).To(BeNil())

		opts := containers.LogOptions{Stdout: &bindings.PTrue, Follow: &bindings.PTrue}
		go func() {
			containers.Logs(bt.conn, r.ID, opts, stdoutChan, nil)
		}()
		o := <-stdoutChan
		o = strings.ReplaceAll(o, "\r", "")
		_, err = time.Parse(time.RFC1123Z, o)
		Expect(err).To(BeNil())
	})
})
