package fuse_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"bazil.org/fuse/fs/fstestutil"
	"github.com/pachyderm/pachyderm/src/client"
	pfsclient "github.com/pachyderm/pachyderm/src/client/pfs"
	"github.com/pachyderm/pachyderm/src/client/pkg/grpcutil"
	"github.com/pachyderm/pachyderm/src/client/pkg/require"
	"github.com/pachyderm/pachyderm/src/client/pkg/shard"
	pfsserver "github.com/pachyderm/pachyderm/src/server/pfs"
	"github.com/pachyderm/pachyderm/src/server/pfs/drive"
	"github.com/pachyderm/pachyderm/src/server/pfs/fuse"
	"github.com/pachyderm/pachyderm/src/server/pfs/server"
	"go.pedge.io/lion"
	"go.pedge.io/pkg/exec"
	"google.golang.org/grpc"
)

func TestRootReadDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		require.NoError(t, c.CreateRepo("one"))
		require.NoError(t, c.CreateRepo("two"))

		require.NoError(t, fstestutil.CheckDir(mountpoint, map[string]fstestutil.FileInfoCheck{
			"one": func(fi os.FileInfo) error {
				if g, e := fi.Mode(), os.ModeDir|0555; g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				// TODO show repoSize in repo stat?
				if g, e := fi.Size(), int64(0); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show RepoInfo.Created as time
				// if g, e := fi.ModTime().UTC(), repoModTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
			"two": func(fi os.FileInfo) error {
				if g, e := fi.Mode(), os.ModeDir|0555; g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				// TODO show repoSize in repo stat?
				if g, e := fi.Size(), int64(0); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show RepoInfo.Created as time
				// if g, e := fi.ModTime().UTC(), repoModTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
		}))
	})
}

func TestRepoReadDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repoName := "foo"
		require.NoError(t, c.CreateRepo(repoName))
		commitA, err := c.StartCommit(repoName, "", "")
		require.NoError(t, err)
		require.NoError(t, c.FinishCommit(repoName, commitA.ID))
		t.Logf("finished commit %v", commitA.ID)

		commitB, err := c.StartCommit(repoName, "", "")
		require.NoError(t, err)
		t.Logf("open commit %v", commitB.ID)

		require.NoError(t, fstestutil.CheckDir(filepath.Join(mountpoint, repoName), map[string]fstestutil.FileInfoCheck{
			commitA.ID: func(fi os.FileInfo) error {
				if g, e := fi.Mode(), os.ModeDir|0555; g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				// TODO show commitSize in commit stat?
				if g, e := fi.Size(), int64(0); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show CommitInfo.StartTime as ctime, CommitInfo.Finished as mtime
				// TODO test ctime via .Sys
				// if g, e := fi.ModTime().UTC(), commitFinishTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
			commitB.ID: func(fi os.FileInfo) error {
				if g, e := fi.Mode(), os.ModeDir|0775; g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				// TODO show commitSize in commit stat?
				if g, e := fi.Size(), int64(0); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show CommitInfo.StartTime as ctime, ??? as mtime
				// TODO test ctime via .Sys
				// if g, e := fi.ModTime().UTC(), commitFinishTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
		}))
	})
}

func TestCommitOpenReadDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repoName := "foo"
		require.NoError(t, c.CreateRepo(repoName))
		commit, err := c.StartCommit(repoName, "", "")
		require.NoError(t, err)
		t.Logf("open commit %v", commit.ID)

		const (
			greetingName = "greeting"
			greeting     = "Hello, world\n"
			greetingPerm = 0644
		)
		require.NoError(t, ioutil.WriteFile(filepath.Join(mountpoint, repoName, commit.ID, greetingName), []byte(greeting), greetingPerm))
		const (
			scriptName = "script"
			script     = "#!/bin/sh\necho foo\n"
			scriptPerm = 0750
		)
		require.NoError(t, ioutil.WriteFile(filepath.Join(mountpoint, repoName, commit.ID, scriptName), []byte(script), scriptPerm))

		require.NoError(t, fstestutil.CheckDir(filepath.Join(mountpoint, repoName, commit.ID), map[string]fstestutil.FileInfoCheck{
			greetingName: func(fi os.FileInfo) error {
				// TODO respect greetingPerm
				if g, e := fi.Mode(), os.FileMode(0666); g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				if g, e := fi.Size(), int64(len(greeting)); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show fileModTime as mtime
				// if g, e := fi.ModTime().UTC(), fileModTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
			scriptName: func(fi os.FileInfo) error {
				// TODO respect scriptPerm
				if g, e := fi.Mode(), os.FileMode(0666); g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				if g, e := fi.Size(), int64(len(script)); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show fileModTime as mtime
				// if g, e := fi.ModTime().UTC(), fileModTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
		}))
	})
}

func TestCommitFinishedReadDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repoName := "foo"
		require.NoError(t, c.CreateRepo(repoName))
		commit, err := c.StartCommit(repoName, "", "")
		require.NoError(t, err)
		t.Logf("open commit %v", commit.ID)

		const (
			greetingName = "greeting"
			greeting     = "Hello, world\n"
			greetingPerm = 0644
		)
		require.NoError(t, ioutil.WriteFile(filepath.Join(mountpoint, repoName, commit.ID, greetingName), []byte(greeting), greetingPerm))
		const (
			scriptName = "script"
			script     = "#!/bin/sh\necho foo\n"
			scriptPerm = 0750
		)
		require.NoError(t, ioutil.WriteFile(filepath.Join(mountpoint, repoName, commit.ID, scriptName), []byte(script), scriptPerm))
		require.NoError(t, c.FinishCommit(repoName, commit.ID))

		require.NoError(t, fstestutil.CheckDir(filepath.Join(mountpoint, repoName, commit.ID), map[string]fstestutil.FileInfoCheck{
			greetingName: func(fi os.FileInfo) error {
				// TODO respect greetingPerm
				if g, e := fi.Mode(), os.FileMode(0666); g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				if g, e := fi.Size(), int64(len(greeting)); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show fileModTime as mtime
				// if g, e := fi.ModTime().UTC(), fileModTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
			scriptName: func(fi os.FileInfo) error {
				// TODO respect scriptPerm
				if g, e := fi.Mode(), os.FileMode(0666); g != e {
					return fmt.Errorf("wrong mode: %v != %v", g, e)
				}
				if g, e := fi.Size(), int64(len(script)); g != e {
					t.Errorf("wrong size: %v != %v", g, e)
				}
				// TODO show fileModTime as mtime
				// if g, e := fi.ModTime().UTC(), fileModTime; g != e {
				// 	t.Errorf("wrong mtime: %v != %v", g, e)
				// }
				return nil
			},
		}))
	})
}

func TestWriteAndRead(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repoName := "foo"
		require.NoError(t, c.CreateRepo(repoName))
		commit, err := c.StartCommit(repoName, "", "")
		require.NoError(t, err)
		greeting := "Hello, world\n"
		filePath := filepath.Join(mountpoint, repoName, commit.ID, "greeting")
		require.NoError(t, ioutil.WriteFile(filePath, []byte(greeting), 0644))
		readGreeting, err := ioutil.ReadFile(filePath)
		require.NoError(t, err)
		require.Equal(t, greeting, string(readGreeting))
		require.NoError(t, c.FinishCommit(repoName, commit.ID))
		data, err := ioutil.ReadFile(filePath)
		require.NoError(t, err)
		require.Equal(t, []byte(greeting), data)
	})
}

func TestBigWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repo := "test"
		require.NoError(t, c.CreateRepo(repo))
		commit, err := c.StartCommit(repo, "", "")
		require.NoError(t, err)
		path := filepath.Join(mountpoint, repo, commit.ID, "file1")
		stdin := strings.NewReader(fmt.Sprintf("yes | tr -d '\\n' | head -c 1000000 > %s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		require.NoError(t, c.FinishCommit(repo, commit.ID))
		data, err := ioutil.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, bytes.Repeat([]byte{'y'}, 1000000), data)
	})
}

func Test296Appends(t *testing.T) {
	lion.SetLevel(lion.LevelDebug)
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repo := "test"
		require.NoError(t, c.CreateRepo(repo))
		commit, err := c.StartCommit(repo, "", "")
		require.NoError(t, err)
		path := filepath.Join(mountpoint, repo, commit.ID, "file")
		stdin := strings.NewReader(fmt.Sprintf("echo 1 >>%s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		stdin = strings.NewReader(fmt.Sprintf("echo 2 >>%s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		require.NoError(t, c.FinishCommit(repo, commit.ID))
		commit2, err := c.StartCommit(repo, commit.ID, "")
		require.NoError(t, err)
		path = filepath.Join(mountpoint, repo, commit2.ID, "file")
		stdin = strings.NewReader(fmt.Sprintf("echo 3 >>%s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		require.NoError(t, c.FinishCommit(repo, commit2.ID))
		data, err := ioutil.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "1\n2\n3\n", string(data))
	})
}

func Test296(t *testing.T) {
	lion.SetLevel(lion.LevelDebug)
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repo := "test"
		require.NoError(t, c.CreateRepo(repo))
		commit, err := c.StartCommit(repo, "", "")
		require.NoError(t, err)
		path := filepath.Join(mountpoint, repo, commit.ID, "file")
		stdin := strings.NewReader(fmt.Sprintf("echo 1 >%s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		stdin = strings.NewReader(fmt.Sprintf("echo 2 >%s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		require.NoError(t, c.FinishCommit(repo, commit.ID))
		commit2, err := c.StartCommit(repo, commit.ID, "")
		require.NoError(t, err)
		path = filepath.Join(mountpoint, repo, commit2.ID, "file")
		stdin = strings.NewReader(fmt.Sprintf("echo 3 >%s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		require.NoError(t, c.FinishCommit(repo, commit2.ID))
		data, err := ioutil.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "3\n", string(data))
	})
}

func TestSpacedWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repo := "test"
		require.NoError(t, c.CreateRepo(repo))
		commit, err := c.StartCommit(repo, "", "")
		require.NoError(t, err)
		path := filepath.Join(mountpoint, repo, commit.ID, "file")
		file, err := os.Create(path)
		require.NoError(t, err)
		_, err = file.Write([]byte("foo"))
		require.NoError(t, err)
		_, err = file.Write([]byte("foo"))
		require.NoError(t, err)
		require.NoError(t, file.Close())
		require.NoError(t, c.FinishCommit(repo, commit.ID))
		data, err := ioutil.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "foofoo", string(data))
	})
}

func TestMountCachingViaWalk(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repo1 := "foo"
		require.NoError(t, c.CreateRepo(repo1))

		// Now if we walk the FS we should see the file.
		// This first act of 'walking' should also initiate the cache

		var filesSeen []interface{}
		walkCallback := func(path string, info os.FileInfo, err error) error {
			tokens := strings.Split(path, "/mnt")
			filesSeen = append(filesSeen, tokens[len(tokens)-1])
			return nil
		}

		err := filepath.Walk(mountpoint, walkCallback)
		require.NoError(t, err)
		require.OneOfEquals(t, "/foo", filesSeen)

		// Now create another repo, and look for it under the mount point
		repo2 := "bar"
		require.NoError(t, c.CreateRepo(repo2))

		// Now if we walk the FS we should see the new file.
		// This now works. But originally (issue #205) this second ls on mac doesn't report the file!

		filesSeen = make([]interface{}, 0)
		err = filepath.Walk(mountpoint, walkCallback)
		require.NoError(t, err)
		require.OneOfEquals(t, "/foo", filesSeen)
		require.OneOfEquals(t, "/bar", filesSeen)

	})
}

func TestMountCachingViaShell(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}

	testFuse(t, func(c client.APIClient, mountpoint string) {
		repo1 := "foo"
		require.NoError(t, c.CreateRepo(repo1))

		// Now if we walk the FS we should see the file.
		// This first act of 'walking' should also initiate the cache

		ls := exec.Command("ls")
		ls.Dir = mountpoint
		out, err := ls.Output()
		require.NoError(t, err)

		require.Equal(t, "foo\n", string(out))

		// Now create another repo, and look for it under the mount point
		repo2 := "bar"
		require.NoError(t, c.CreateRepo(repo2))

		// Now if we walk the FS we should see the new file.
		// This second ls on mac doesn't report the file!

		ls = exec.Command("ls")
		ls.Dir = mountpoint
		out, err = ls.Output()
		require.NoError(t, err)

		require.Equal(t, true, "foo\nbar\n" == string(out) || "bar\nfoo\n" == string(out))

	})
}

func TestCreateFileInDir(t *testing.T) {
	lion.SetLevel(lion.LevelDebug)
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}
	testFuse(t, func(c client.APIClient, mountpoint string) {
		require.NoError(t, c.CreateRepo("repo"))
		commit, err := c.StartCommit("repo", "", "")
		require.NoError(t, err)

		require.NoError(t, os.Mkdir(filepath.Join(mountpoint, "repo", commit.ID, "dir"), 0700))
		require.NoError(t, ioutil.WriteFile(filepath.Join(mountpoint, "repo", commit.ID, "dir", "file"), []byte("foo"), 0644))
		require.NoError(t, c.FinishCommit("repo", commit.ID))
	})
}

func TestOverwriteFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipped because of short mode")
	}
	testFuse(t, func(c client.APIClient, mountpoint string) {
		require.NoError(t, c.CreateRepo("repo"))
		commit1, err := c.StartCommit("repo", "", "")
		require.NoError(t, err)
		_, err = c.PutFile("repo", commit1.ID, "file", strings.NewReader("foo\n"))
		require.NoError(t, err)
		require.NoError(t, c.FinishCommit("repo", commit1.ID))
		commit2, err := c.StartCommit("repo", commit1.ID, "")
		require.NoError(t, err)
		path := filepath.Join(mountpoint, "repo", commit2.ID, "file")
		stdin := strings.NewReader(fmt.Sprintf("echo bar >%s", path))
		require.NoError(t, pkgexec.RunStdin(stdin, "sh"))
		require.NoError(t, c.FinishCommit("repo", commit2.ID))
		result, err := ioutil.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "bar\n", string(result))
	})
}

func testFuse(
	t *testing.T,
	test func(client client.APIClient, mountpoint string),
) {
	// don't leave goroutines running
	var wg sync.WaitGroup
	defer wg.Wait()

	tmp, err := ioutil.TempDir("", "pachyderm-test-")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmp)
	}()

	// closed on successful termination
	quit := make(chan struct{})
	defer close(quit)
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()

	// TODO try to share more of this setup code with various main
	// functions
	localAddress := listener.Addr().String()
	srv := grpc.NewServer()
	const (
		numShards = 1
	)
	sharder := shard.NewLocalSharder([]string{localAddress}, numShards)
	hasher := pfsserver.NewHasher(numShards, 1)
	router := shard.NewRouter(
		sharder,
		grpcutil.NewDialer(
			grpc.WithInsecure(),
		),
		localAddress,
	)

	blockDir := filepath.Join(tmp, "blocks")
	blockServer, err := server.NewLocalBlockAPIServer(blockDir)
	require.NoError(t, err)
	pfsclient.RegisterBlockAPIServer(srv, blockServer)

	driver, err := drive.NewDriver(localAddress)
	require.NoError(t, err)

	apiServer := server.NewAPIServer(
		hasher,
		router,
	)
	pfsclient.RegisterAPIServer(srv, apiServer)

	internalAPIServer := server.NewInternalAPIServer(
		hasher,
		router,
		driver,
	)
	pfsclient.RegisterInternalAPIServer(srv, internalAPIServer)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Serve(listener); err != nil {
			select {
			case <-quit:
				// orderly shutdown
				return
			default:
				t.Errorf("grpc serve: %v", err)
			}
		}
	}()

	clientConn, err := grpc.Dial(localAddress, grpc.WithInsecure())
	require.NoError(t, err)
	apiClient := pfsclient.NewAPIClient(clientConn)
	mounter := fuse.NewMounter(localAddress, apiClient)
	mountpoint := filepath.Join(tmp, "mnt")
	require.NoError(t, os.Mkdir(mountpoint, 0700))
	ready := make(chan bool)
	wg.Add(1)
	go func() {
		defer wg.Done()
		require.NoError(t, mounter.MountAndCreate(mountpoint, nil, nil, ready))
	}()

	<-ready

	defer func() {
		_ = mounter.Unmount(mountpoint)
	}()
	test(client.APIClient{PfsAPIClient: apiClient}, mountpoint)
}
