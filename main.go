package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
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
	//Open file to track directory paths
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
	return fmt.Sprintf("%s/%s/%s/%s", b[:2], b[2:4], b[4:6], b[6:])
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
			return nil
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

		// Write name to dirpath file
		dir := filepath.Dir(newPath)
		_, err = f.WriteString(dir + "\n")
		if err != nil {
			fmt.Println("unable to write dir to file", dir)
		}

		// Create directory
		err = os.MkdirAll(dir, modules.DefaultDirPerm)
		if err != nil {
			return errors.AddContext(err, "os.MkdirAll  failed")
		}

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
		if len(elements) != 3 {
			return false
		}
		for _, el := range elements {
			if len(el) != 2 {
				return false
			}
		}
		return true
	}

	// Handle filepath
	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, "/")
	elements := strings.Split(path, "/")
	if len(elements) != 4 {
		return false
	}
	for i, el := range elements {
		if i == 3 {
			break
		}
		if len(el) != 2 {
			return false
		}
	}
	return true
}
