package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"io/ioutil"

	"github.com/docker/go-plugins-helpers/volume"
)

const (
	sshfsId       = "_sshfs"
	socketAddress = "/run/docker/plugins/sshfs.sock"
)

var (
	defaultDir = filepath.Join(volume.DefaultDockerRootDirectory, sshfsId)
	root       = flag.String("root", defaultDir, "SshFS volumes root directory")
)

type volume_name struct {
	name        string
	connections int
}

type sshfsDriver struct {
	sync.Mutex

	root    string
	volumes map[string]*volume_name
}

func newSshfsDriver(root string) sshfsDriver {
	d := sshfsDriver{
		root:    root,
		volumes: map[string]*volume_name{},
	}

	return d
}

func (d sshfsDriver) Create(r volume.Request) volume.Response {
	return volume.Response{}
}

func (d sshfsDriver) Remove(r volume.Request) volume.Response {
	d.Lock()
	defer d.Unlock()
	m := d.mountpoint(r.Name)

	if s, ok := d.volumes[m]; ok {
		if s.connections <= 1 {
			delete(d.volumes, m)
		}
	}
	return volume.Response{}
}

func (d sshfsDriver) Path(r volume.Request) volume.Response {
	return volume.Response{Mountpoint: d.mountpoint(r.Name)}
}

func (d sshfsDriver) Mount(r volume.Request) volume.Response {
	d.Lock()
	defer d.Unlock()
	m := d.mountpoint(r.Name)

	s, ok := d.volumes[m]
	if ok && s.connections > 0 {
		log.Printf("Volume %s already mounted on %s\n", r.Name, m)
		s.connections++
		return volume.Response{Mountpoint: m}
	}

	fi, err := os.Lstat(m)

	if os.IsNotExist(err) {
		log.Printf("Volume mountpoint %s does not exist, creating\n", m)
		if err := os.MkdirAll(m, 0755); err != nil {
			log.Printf("Creating volume mountpoint %s failed\n", m)
			return volume.Response{Err: err.Error()}
		}
	} else if err != nil {
		log.Printf("Volume mountpoint %s exists but os.Lstat returned error %s\n", m, err.Error())
		return volume.Response{Err: err.Error()}
	}

	if fi != nil && !fi.IsDir() {
		return volume.Response{Err: fmt.Sprintf("%v already exist and it's not a directory\n", m)}
	}

	if err := d.mountVolume(r.Name, m); err != nil {
		log.Printf("Mounting volume %s failed with error %s\n", m, err.Error())
		return volume.Response{Err: err.Error()}
	}

	d.volumes[m] = &volume_name{name: r.Name, connections: 1}

	log.Printf("Mounting volume %s on %s\n", r.Name, m)
	return volume.Response{Mountpoint: m}
}

func (d sshfsDriver) Unmount(r volume.Request) volume.Response {
	d.Lock()
	defer d.Unlock()
	m := d.mountpoint(r.Name)

	if s, ok := d.volumes[m]; ok {
		if s.connections == 1 {
			log.Printf("Unmounting volume %s from %s\n", r.Name, m)
			if err := d.unmountVolume(m); err != nil {
				return volume.Response{Err: err.Error()}
			}
		}
		s.connections--
	} else {
		return volume.Response{Err: fmt.Sprintf("Unable to find volume mounted on %s", m)}
	}

	return volume.Response{}
}

func (d sshfsDriver) Get(r volume.Request) volume.Response {
	d.Lock()
	defer d.Unlock()
	m := d.mountpoint(r.Name)
	if s, ok := d.volumes[m]; ok {
		return volume.Response{Volume: &volume.Volume{Name: s.name, Mountpoint: d.mountpoint(s.name)}}
	}

	return volume.Response{Err: fmt.Sprintf("Unable to find volume mounted on %s", m)}
}

func (d sshfsDriver) List(r volume.Request) volume.Response {
	d.Lock()
	defer d.Unlock()
	var vols []*volume.Volume
	for _, v := range d.volumes {
		vols = append(vols, &volume.Volume{Name: v.name, Mountpoint: d.mountpoint(v.name)})
	}
	return volume.Response{Volumes: vols}
}

func (d sshfsDriver) Capabilities(r volume.Request) volume.Response {
	return volume.Response{Capabilities: volume.Capability{Scope: "global"}}
}

func (d *sshfsDriver) mountpoint(name string) string {
	return filepath.Join(d.root, name)
}

func (d *sshfsDriver) mountVolume(name, destination string) error {
	parts := strings.Split(name, "#")
	if len(parts) == 1 {
		name = name + ":/"
	} else if len(parts) == 2 {
		name = parts[0] + ":" + parts[1]
	} else {
		return fmt.Errorf("invalid name, use [user@]host#[dir]")
	}
	cmd := fmt.Sprintf("sshfs %s  %s", name, destination)
        log.Printf("Attempting to mount %s to %s using command %s\n", name, destination, cmd)
	ecmd := exec.Command("sh", "-c", cmd)
        stderr, err := ecmd.StderrPipe()
	if err != nil {
		log.Printf("Unable to open stderr before running mount command: %s\n", err.Error())
		return err
	}
	if err := ecmd.Start(); err != nil {
		log.Printf("Mount command failed when starting it: %s\n", err)
		return err
	}
	if err := ecmd.Wait(); err != nil {
		log.Printf("Mount command failed while waiting for it to complete: %s\n", err.Error())
		errstr, readerr :=  ioutil.ReadAll(stderr)
		if readerr != nil {
                  log.Printf("Reading stderr of failed command failed with error %s\n", readerr.Error())
		}
		log.Printf("Command stderr output: %s\n", errstr)
		return err
	}
	return nil
}

func (d *sshfsDriver) unmountVolume(target string) error {
	cmd := fmt.Sprintf("umount %s", target)
	return exec.Command("sh", "-c", cmd).Run()
}

func main() {
	flag.Parse()

	d := newSshfsDriver(*root)
	h := volume.NewHandler(d)
	fmt.Printf("listening on %s\n", socketAddress)
	fmt.Println(h.ServeUnix("root", socketAddress))
}
