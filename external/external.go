package external

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	configurationFolder = "conf/active/"
	backupFolder        = "content/backup/"
	contentFolder       = "content/"
)

// Resource describes an external (distant) resource
// UpdateInterval is a duration describing how often the resource should be fetched
// FriendlyName is the name given to the resource. It is used to create the backup folder,
// and to display friendly logs (without the file name, backup folder name, etc...)
type Resource struct {
	UpdateInterval time.Duration
	FriendlyName   string
	FileName       string
	FullPath       string
	URL            string
	Iterations     int
}

// UnparsedResource is used to store the yaml representation of the resource.
// This is a temporary structure.
type UnparsedResource struct {
	UpdateInterval string
	FriendlyName   string
	URL            string
	FileName       string
}

// AvailableResources is a map that contains
var AvailableResources = make(map[string]*Resource)

// LoadAndStart loads all the active configurations and start the goroutines.
func LoadAndStart() error {
	files, err := ioutil.ReadDir(configurationFolder)
	if err != nil {
		log.Println("Could not read configuration folder :", err)
		return err
	}
	for _, fd := range files {
		log.Println("Loading Configuration :", fd.Name())
		ext, err := LoadConfiguration(configurationFolder + fd.Name())
		if err != nil {
			log.Printf("Error with file %v. It won't be used. %v\n", fd.Name(), err)
			continue
		}
		log.Printf("Starting External Resource Collection for %v (%v)\n", ext.FriendlyName, fd.Name())
		go ext.PeriodicUpdate()
	}
	return nil
}

// LoadConfiguration reads the configuration file and returns an ExternalResource
func LoadConfiguration(configPath string) (Resource, error) {
	conf, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Println("Could not read external resource configuration :", err)
		return Resource{}, err
	}
	unparsedExternal := new(UnparsedResource)
	err = yaml.Unmarshal(conf, &unparsedExternal)
	if err != nil {
		log.Println("Error parsing YAML :", err)
		return Resource{}, err
	}
	duration, err := time.ParseDuration(unparsedExternal.UpdateInterval)
	if err != nil {
		log.Println("Error parsing Duration :", err)
		return Resource{}, err
	}
	external := Resource{
		UpdateInterval: duration,
		FriendlyName:   unparsedExternal.FriendlyName,
		URL:            unparsedExternal.URL,
		FileName:       unparsedExternal.FileName,
		FullPath:       contentFolder + unparsedExternal.FileName,
	}
	return external, nil
}

// CalculateIteration parses the backup folder for an ExternalResource and
// determines the highest iteration by reading the file names.
func (ext *Resource) CalculateIteration(backupFolder string) error {
	files, err := ioutil.ReadDir(backupFolder)
	if err != nil {
		return err
	}
	toplevel := 0
	if len(files) > 0 {
		for _, f := range files {
			level, err := strconv.Atoi(f.Name()[len(f.Name())-1:])
			if err != nil {
				log.Printf("Could not calculate Iteration for %v. It will be set to 0.\n", ext.FriendlyName)
				ext.Iterations = 0
				return nil
			}
			if level > toplevel {
				toplevel = level
			}
		}
		ext.Iterations = toplevel + 1
		return nil
	}
	ext.Iterations = toplevel
	return nil
}

// PeriodicUpdate starts the periodic update for the Resource.
func (ext *Resource) PeriodicUpdate() {
	tmpFileName := contentFolder + ext.FileName + ".tmp"
	currentFileName := contentFolder + ext.FileName
	mapName := ext.FileName[0 : len(ext.FileName)-len(filepath.Ext(ext.FileName))]
	specificBackupFolder := backupFolder + mapName + "/"

	if err := CheckAndCreateFolder(contentFolder); err != nil {
		log.Println(err)
		return
	}

	if err := CheckAndCreateFolder(backupFolder); err != nil {
		log.Println(err)
		return
	}

	if err := CheckAndCreateFolder(specificBackupFolder); err != nil {
		log.Println(err)
		return
	}

	if err := ext.CalculateIteration(specificBackupFolder); err != nil {
		log.Println("Error calculating Iteration :", err)
		return
	}

	if err := DownloadNamedFile(ext.URL, ext.FullPath); err != nil {
		log.Println("Error dowloading file :", err)
		return
	}

	tickChan := time.NewTicker(ext.UpdateInterval).C

	log.Println("Resource Collection Started for", ext.FriendlyName)
	AvailableResources[mapName] = ext
	log.Printf("Added Available Resource %v as %v\n", ext.FriendlyName, mapName)

	for {
		<-tickChan

		if err := DownloadNamedFile(ext.URL, tmpFileName); err != nil {
			log.Println(err)
			continue
		}

		same, err := SameFileCheck(currentFileName, tmpFileName)
		if err != nil {
			log.Println(err)
			continue
		}

		if same {
			if err := os.Remove(tmpFileName); err != nil {
				log.Println(err)
				continue
			}
		} else {
			if err := os.Rename(currentFileName, specificBackupFolder+ext.FileName+"."+strconv.Itoa(ext.Iterations)); err != nil {
				log.Println(err)
				continue
			}
			ext.Iterations++
			if err := os.Rename(tmpFileName, currentFileName); err != nil {
				log.Println(err)
				continue
			}
			log.Printf("New content in %v. Downloaded and replaced.\n", ext.FriendlyName)
		}
	}
}