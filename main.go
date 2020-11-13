package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/modules/renter/filesystem/siadir"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
)

const (
	// dirDepth and dirLength are used to define the desired filesystem structure.
	//
	// For Example, if dirDepth is 3 and dirLength is 2 we will get filepaths of
	// the structure aa/aa/aa/filename
	dirDepth  = 3
	dirLength = 2
)

var (
	dirs = make(map[string]struct{})
)

// copyFile will copy a file on disk to a new location and remove the old file
func copyFile(oldPath, newPath string) error {
	// Read data from file at old path
	data, err := ioutil.ReadFile(oldPath)
	if err != nil {
		return errors.AddContext(err, "ioutil.ReadFile failed")
	}
	// Write to new extended file path
	err = ioutil.WriteFile(newPath, data, modules.DefaultFilePerm)
	if err != nil {
		return errors.AddContext(err, "ioutil.WriteFile failed")
	}
	// Remove old extended file path
	err = os.Remove(oldPath)
	if err != nil {
		return errors.AddContext(err, "os.Remove failed failed")
	}
	return nil
}

// deleteEmptyDirs will walk the filesystem and delete an empty directories. It
// will delete them recursively, so if one deletion creates a new empty
// directory, then that directory should be deleted as well.
//
// NOTE: a directory that only contains a .siadir file is considered empty
func deleteEmptyDirs(root string) error {
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		// Check if fi is nil, this happens when the .siadir is deleted from the
		// directory and walk expects to visit it next
		if fi == nil {
			return nil
		}
		// Ignore files
		if !fi.IsDir() {
			return nil
		}

		// Recursively delete empty directories
		return recurviseDelete(path)
	})
}

func main() {
	// Check input args
	args := os.Args
	fmt.Println(args)
	switch len(args) {
	case 1:
		fmt.Println("Executing Rename and Delete")
	case 2:
		if args[1] != "delete-only" {
			panic("Improper use")
		}
		fmt.Println("Executing Delete Only")
		err := deleteEmptyDirs("./fs/var/skynet")
		if err != nil {
			println("error deleting dirs", err)
			os.Exit(1)
		}
		println("Deletion Done")
		return
	default:
		panic("Improper use")
	}

	// Open file to track directory paths
	f, err := os.OpenFile("dirpaths", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		println("error creating dirpath file", err)
		os.Exit(1)
	}
	defer func() {
		if err := f.Close(); err != nil {
			println(err)
		}
	}()

	// Rename Files
	err = renameAll(f, "./fs/var/skynet")
	if err != nil {
		println("error renaming files", err)
		os.Exit(1)
	}

	// Go back over the file system and delete any empty directories
	err = deleteEmptyDirs("./fs/var/skynet")
	if err != nil {
		println("error deleting dirs", err)
		os.Exit(1)
	}
}

// randomName returns a random file name following a 2/2/2/26 structure
func randomName() string {
	b := hex.EncodeToString((fastrand.Bytes(16)))
	str := ""
	for i := 0; i < dirDepth; i++ {
		if i == 0 {
			str = fmt.Sprintf("%s", b[:dirLength])
			continue
		}
		str = fmt.Sprintf("%s/%s", str, b[dirLength*i:dirLength*(i+1)])
	}
	str = fmt.Sprintf("%s/%s", str, b[dirLength*dirDepth:])
	return str
}

// recurviseDelete will delete all directories for a given path that are empty
// starting with the lowest level child directory
func recurviseDelete(path string) error {
	for path != "." && path != "/" {
		// Read directory
		fileinfos, err := ioutil.ReadDir(path)
		if err != nil {
			return errors.AddContext(err, fmt.Sprintf("unable to read dir %s", path))
		}
		// If the dir is not empty we return
		if len(fileinfos) > 1 {
			return nil
		}
		if len(fileinfos) == 1 {
			fi := fileinfos[0]
			if fi.Name() != ".siadir" {
				return nil
			}
			siadir := filepath.Join(path, fi.Name())
			// Attempt to delete the .siadir file
			err = os.Remove(siadir)
			if err != nil && !os.IsNotExist(err) {
				return errors.AddContext(err, fmt.Sprintf("unable to remove siadir %s", siadir))
			}
		}

		// Delete empty directory
		err = os.Remove(path)
		if err != nil {
			return errors.AddContext(err, fmt.Sprintf("unable to remove path %s", path))
		}

		// Get parent directory
		path = filepath.Dir(path)
	}
	return nil
}

// createSiaDir creates a siadir on disk if there is not one present
func createSiaDir(dir string) error {
	path := filepath.Join(dir, modules.SiaDirExtension)
	// Check for existing siadir
	_, err := os.Stat(path)
	if !os.IsNotExist(err) {
		return nil
	}

	// Create Metadata
	now := time.Now()
	md := siadir.Metadata{
		AggregateHealth:        siadir.DefaultDirHealth,
		AggregateMinRedundancy: siadir.DefaultDirRedundancy,
		AggregateModTime:       now,
		AggregateRemoteHealth:  siadir.DefaultDirHealth,
		AggregateStuckHealth:   siadir.DefaultDirHealth,

		Health:        siadir.DefaultDirHealth,
		MinRedundancy: siadir.DefaultDirRedundancy,
		Mode:          os.FileMode(modules.DefaultDirPerm),
		ModTime:       now,
		RemoteHealth:  siadir.DefaultDirHealth,
		StuckHealth:   siadir.DefaultDirHealth,
	}

	// Marshal the Metadata
	data, err := json.Marshal(md)
	if err != nil {
		return errors.AddContext(err, "unable to marshal metadata")
	}

	// Write the data to disk and sync
	file, err := os.OpenFile(path, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Compose(err, file.Close())
	}()

	// Write and sync.
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n < len(data) {
		return fmt.Errorf("write was only applied partially - %v / %v", n, len(data))
	}
	return file.Sync()

}

// renameAll will walk the filesystem and rename all files to create a directory
// structure that follows a 2/2/2/26 pattern
func renameAll(f *os.File, root string) error {
	totalFiles := 0
	// Loop over files and rename them
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		// Ignore non siafiles and dirs.
		ext := filepath.Ext(fi.Name())
		if ext != modules.SiaFileExtension {
			return nil
		}
		if fi.IsDir() {
			return nil
		}
		totalFiles++
		if totalFiles%1000 == 0 {
			println(totalFiles, "files handled")
		}
		// Ignore files already in the 2/2/2/<filename> structure
		dirStructure := strings.TrimPrefix(path, root)
		if validDirStructure(dirStructure) {
			// Verify there is a siadir in this directory
			dir := filepath.Dir(path)
			return createSiaDir(dir)
		}

		// Log Original name
		name := strings.TrimSuffix(path, modules.SiaFileExtension)

		// Ignore extended files
		if strings.HasSuffix(name, "-extended") {
			return nil
		}

		// Determine new paths
		newName := randomName() + modules.SiaFileExtension
		newPath := filepath.Join(root, newName)
		oldPathExtended := strings.TrimSuffix(name, modules.SiaFileExtension) + "-extended" + modules.SiaFileExtension
		newPathExtended := strings.TrimSuffix(newPath, modules.SiaFileExtension) + "-extended" + modules.SiaFileExtension

		// Check if this is a new directory
		dir := filepath.Dir(newPath)
		_, exists := dirs[dir]
		if !exists {
			// Write name to dirpath file
			_, err = f.WriteString(dir + "\n")
			if err != nil {
				fmt.Println("unable to write dir to file", dir)
			}

			// Create directory
			err = os.MkdirAll(dir, modules.DefaultDirPerm)
			if err != nil {
				return errors.AddContext(err, "os.MkdirAll  failed")
			}

			// Create a SiaDir file
			err = createSiaDir(dir)
			if err != nil {
				return errors.AddContext(err, "createSiaDir  failed")
			}
		}

		// Add to dirs map, it is fine if we are overwriting an existing entry
		dirs[dir] = struct{}{}

		// Ignore edge case that file exists are new location
		if path == newPath {
			return nil
		}

		// Copy the siafile
		err = copyFile(path, newPath)
		if err != nil {
			return errors.AddContext(err, "copyFile  failed")
		}

		// If there is not an extended file we are done
		_, err = os.Stat(oldPathExtended)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return errors.AddContext(err, "os.Stat for extended failed")
		}

		// Copy the extended file
		err = copyFile(oldPathExtended, newPathExtended)
		if err != nil {
			return errors.AddContext(err, "copyFile for extended failed")
		}
		return nil
	})
}

// validDirStructure returns a boolean indicating if the path is of the
// structure /2/2/2/<filename>.
func validDirStructure(path string) bool {
	// Check for directory
	dir, file := filepath.Split(path)
	if file == "" {
		path = filepath.Clean(dir)
		path = strings.TrimPrefix(path, "/")
		elements := strings.Split(path, "/")
		if len(elements) != dirDepth {
			return false
		}
		for _, el := range elements {
			if len(el) != dirLength {
				return false
			}
		}
		return true
	}

	// Handle filepath
	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, "/")
	elements := strings.Split(path, "/")
	if len(elements) != dirDepth+1 {
		return false
	}
	for i, el := range elements {
		if i == dirDepth {
			break
		}
		if len(el) != dirLength {
			return false
		}
	}
	return true
}
