package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/modules/renter/filesystem/siadir"
	"gitlab.com/NebulousLabs/Sia/persist"
	"gitlab.com/NebulousLabs/fastrand"
)

// tempDir creates a temporary directory for testing
func tempDir(name string) string {
	path := filepath.Join(os.TempDir(), name)
	err := os.RemoveAll(path)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(path, persist.DefaultDiskPermissionsTest)
	if err != nil {
		panic(err)
	}
	return path
}

// TestCopyFile tests the copy
func TestCopyFile(t *testing.T) {
	// Create the temp dir for the test
	testDir := tempDir(t.Name())

	// Create a file and write data to it
	name := filepath.Join(testDir, "testfile.dat")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	data := fastrand.Bytes(100)
	_, err = f.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Copy file to a new destination
	newname := filepath.Join(testDir, "newname.dat")
	err = copyFile(name, newname)
	if err != nil {
		t.Fatal(err)
	}

	// Read file at new destination and verify data
	newData, err := ioutil.ReadFile(newname)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, newData) {
		t.Fatal("bad")
	}
}

// TestRandomName tests the random name generation satisfies the
// validDirStructure
func TestRandomName(t *testing.T) {
	for i := 0; i < 1000; i++ {
		if !validDirStructure(randomName()) {
			t.Fatal("bad")
		}
	}
}

// TestRecursiveDelete tests the recursive deletion
func TestRecursiveDelete(t *testing.T) {
	// Create directory tree
	testDir := tempDir(t.Name())
	path := filepath.Join(testDir, "a/s/d/f/s/s")
	err := os.MkdirAll(path, persist.DefaultDiskPermissionsTest)
	if err != nil {
		t.Fatal(err)
	}

	// Delete the directory tree and verify it is removed from disk
	err = recurviseDelete(path)
	if err != nil {
		t.Fatal(err)
	}
	for path != "." && path != "/" && path != "/tmp" {
		_, err = os.Stat(path)
		if !os.IsNotExist(err) {
			t.Fatal(err, path)
		}
		path = filepath.Dir(path)
	}

	// Create directory tree that shouldn't be fully deleted
	testDir = tempDir(t.Name())
	path = filepath.Join(testDir, "a/s/d/f/s/s")
	path2 := filepath.Join(testDir, "a/s/d/e")
	dir2 := filepath.Dir(path2)
	err = os.MkdirAll(path, persist.DefaultDiskPermissionsTest)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(path2, persist.DefaultDiskPermissionsTest)
	if err != nil {
		t.Fatal(err)
	}

	// Delete the directory tree and verify that only the expected directories
	// were deleted
	err = recurviseDelete(path)
	if err != nil {
		t.Fatal(err)
	}
	for path != "." && path != "/" {
		_, err = os.Stat(path)
		if path == dir2 {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if !os.IsNotExist(err) {
			t.Fatal(err, path)
		}
		path = filepath.Dir(path)
	}
}

// TestValidDirStructure tests the validDirStructure function
func TestValidDirStructure(t *testing.T) {
	var tests = []struct {
		path  string
		valid bool
	}{
		{"/", false},
		{"/a/a/a/name", false},
		{"/name", false},
		{"/a/a/a/a/a/a/name", false},
		{"/aa/aa/aa/name/", false},
		// {"/aa/aa/aa/name", true},
		// {"./aa/aa/aa/name", true},
		// {"aa/aa/aa/name", true},
		// {"//////aa/aa/aa/name", true},
		// {"/aa/aa//////aa/name", true},
		{"/aa/aa/name", true},
		{"./aa/aa/name", true},
		{"aa/aa/name", true},
		{"//////aa/aa/name", true},
		{"/aa//////aa/name", true},
	}
	for _, test := range tests {
		if test.valid != validDirStructure(test.path) {
			t.Error("bad", test)
		}
	}
}

// TestRenameAllAndDelete tests the full implementation of renaming an entire
// directory system and deleting the empty directories
func TestRenameAllAndDelete(t *testing.T) {
	// Create a testing directory and directory system
	testDir := tempDir(t.Name())
	fileDir := filepath.Join(testDir, "files")
	files := []string{
		filepath.Join(fileDir, "file.sia"),
		filepath.Join(fileDir, "file2.sia"),
		filepath.Join(fileDir, "file3.sia"),
		filepath.Join(fileDir, "file3-extended.sia"),
		filepath.Join(fileDir, "a/file.sia"),
		filepath.Join(fileDir, "a/file-extended.sia"),
		filepath.Join(fileDir, "a/file2.sia"),
		filepath.Join(fileDir, "a/a/a/file.sia"),
		filepath.Join(fileDir, "/as/as/as/as/as/file.sia"),
	}
	siadirs := []string{
		filepath.Join(fileDir, "a/.siadir"),
		filepath.Join(fileDir, "a/a/a/.siadir"),
	}
	goodFile := filepath.Join(fileDir, "bb/bb/file.sia")
	err := os.MkdirAll(filepath.Dir(goodFile), persist.DefaultDiskPermissionsTest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Create(goodFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		err = os.MkdirAll(filepath.Dir(file), persist.DefaultDiskPermissionsTest)
		if err != nil {
			t.Fatal(err)
		}
		_, err = os.Create(file)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, siadir := range siadirs {
		err = os.MkdirAll(filepath.Dir(siadir), persist.DefaultDiskPermissionsTest)
		if err != nil {
			t.Fatal(err)
		}
		_, err = os.Create(siadir)
		if err != nil {
			t.Fatal(err)
		}
	}

	// rename files
	dirFile := filepath.Join(testDir, "File")
	f, err := os.Create(dirFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	err = renameAll(f, fileDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify renaming
	_, err = os.Stat(goodFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		_, err = os.Stat(file)
		if !os.IsNotExist(err) {
			t.Fatal(err, file)
		}
	}

	// Delete all the empty dirs
	err = deleteEmptyDirs(fileDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		dir, _ := filepath.Split(file)
		dir = strings.TrimSuffix(dir, "/")
		if dir == fileDir {
			continue
		}
		_, err = os.Stat(dir)
		if !os.IsNotExist(err) {
			t.Fatal(err, dir)
		}
	}
	for _, siadir := range siadirs {
		_, err = os.Stat(siadir)
		if !os.IsNotExist(err) {
			t.Fatal(err, siadir)
		}
	}
}

// TestCreateSiaDir tests the createSiaDir function
func TestCreateSiaDir(t *testing.T) {
	// Create test directory
	testDir := tempDir(t.Name())

	// Make a siadir on disk
	err := createSiaDir(testDir)
	if err != nil {
		t.Fatal(err)
	}

	// Load the siadir from disk
	var md siadir.Metadata
	file, err := os.Open(filepath.Join(testDir, modules.SiaDirExtension))
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(bytes, &md)
	if err != nil {
		t.Fatal(err)
	}

	// Verify metadata
	if md.AggregateHealth != siadir.DefaultDirHealth {
		t.Fatal("bad")
	}
	if md.AggregateMinRedundancy != siadir.DefaultDirRedundancy {
		t.Fatal("bad")
	}
	if md.AggregateRemoteHealth != siadir.DefaultDirHealth {
		t.Fatal("bad")
	}
	if md.AggregateStuckHealth != siadir.DefaultDirHealth {
		t.Fatal("bad")
	}
	if md.Health != siadir.DefaultDirHealth {
		t.Fatal("bad")
	}
	if md.MinRedundancy != siadir.DefaultDirRedundancy {
		t.Fatal("bad")
	}
	if md.Mode != os.FileMode(modules.DefaultDirPerm) {
		t.Fatal("bad")
	}
	if md.RemoteHealth != siadir.DefaultDirHealth {
		t.Fatal("bad")
	}
	if md.StuckHealth != siadir.DefaultDirHealth {
		t.Fatal("bad")
	}

	// Grab Mod Times
	amt := md.AggregateModTime
	mt := md.ModTime
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	md = siadir.Metadata{}

	// verify a call to createSiaDir is a no-op
	err = createSiaDir(testDir)
	if err != nil {
		t.Fatal(err)
	}

	// Load the File again and verify the times have not changed
	file, err = os.Open(filepath.Join(testDir, modules.SiaDirExtension))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	bytes, err = ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(bytes, &md)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Mod Times have not changed
	if !mt.Equal(md.ModTime) {
		t.Fatal("bad")
	}
	if !amt.Equal(md.AggregateModTime) {
		t.Fatal("bad")
	}
}
