package main

import (
	"context"
	"flag"
	"fmt"
	pb "gopkg.in/cheggaaa/pb.v1"
	"image"
	"image/gif"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/andybons/gogif"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

const (
	VERSION = "1.0"
)

// Converts an image to an image.Paletted with 256 colors.
func imageToPaletted(img image.Image) *image.Paletted {
	pm, ok := img.(*image.Paletted)
	if !ok {
		b := img.Bounds()
		pm = image.NewPaletted(b, nil)
		q := &gogif.MedianCutQuantizer{NumColor: 256}
		q.Quantize(pm, b, img, image.ZP)
	}
	return pm
}

func processJpeg(path string) (error, *image.Paletted) {
	f, err := os.Open(path)
	if err != nil {
		logrus.WithFields(logrus.Fields{"error": err, "file": path}).Error("While opening file")
		return err, nil
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		logrus.WithFields(logrus.Fields{"error": err, "file": path}).Error("while decoding file")
		return err, nil
	}

	return nil, imageToPaletted(img)
}

func main() {
	verbose := flag.Bool("d", false, "debug mode")
	outfile := flag.String("o", "output.gif", "name of the output gif file name")
	delayMs := flag.Uint("t", 100, "delay (ms) between successive images")
	version := flag.Bool("v", false, "print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "%s <options> dirname\n", os.Args[0])
		fmt.Fprint(flag.CommandLine.Output(), "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *verbose {
		fmt.Println("Starting in debug mode...")
		logrus.SetLevel(logrus.DebugLevel)
	}

	if *version {
		fmt.Printf("%s -- v%s\n", os.Args[0], VERSION)
		return
	}

	_, err := os.Stat(*outfile)
	if !os.IsNotExist(err) {
		logrus.WithFields(logrus.Fields{"file": *outfile}).Error("output file already exists")
		return
	}

	args := flag.Args()
	if len(args) != 1 {
		logrus.Error("you need to specify a single argument as directory")
		return
	}
	dirname := args[0]

	var imgPaths []string
	err = filepath.Walk(dirname, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			logrus.Debugf("skipping dir %s", path)
			return nil
		}
		extension := strings.TrimPrefix(filepath.Ext(path), ".")
		if !(strings.EqualFold(extension, "jpg") || strings.EqualFold(extension, "jpeg")) {
			logrus.Debug("Skipping non jpeg file")
			return nil
		}
		logrus.WithFields(logrus.Fields{"file": path}).Debug("found file")
		imgPaths = append(imgPaths, path)
		return nil
	})

	if err != nil {
		logrus.WithField("err", err).Errorf("error while looking for jpeg files")
		return
	}

	var mutex sync.Mutex
	gifInfo := &gif.GIF{}
	gifInfo.Image = make([]*image.Paletted, len(imgPaths))
	gifInfo.Delay = make([]int, len(imgPaths))

	var wg sync.WaitGroup
	numcpus := runtime.NumCPU()
	sem := semaphore.NewWeighted(int64(numcpus))

	logrus.WithFields(logrus.Fields{
		"number":  len(imgPaths),
		"// jobs": numcpus,
	}).Info("Parallel processing jpeg files")

	bar := pb.New(len(imgPaths))
	bar.SetMaxWidth(80)
	bar.Start()

	for i, jpeg := range imgPaths {
		wg.Add(1)
		go func(jpeg string, i int) {
			defer wg.Done()
			_ = sem.Acquire(context.Background(), 1)
			defer sem.Release(1)
			logrus.WithField("file", jpeg).Debug("processing")

			err, frame := processJpeg(jpeg)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
					"file":  jpeg}).Error("while processing jpeg file")
			}
			mutex.Lock()
			gifInfo.Image[i] = frame
			gifInfo.Delay[i] = int(*delayMs / 10)
			bar.Increment()
			mutex.Unlock()
		}(jpeg, i)
	}
	wg.Wait()
	bar.Finish()

	gifFile, err := os.OpenFile(*outfile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		logrus.WithField("error", err).Error("While creating gif file")
		return
	}

	defer gifFile.Close()
	if err := gif.EncodeAll(gifFile, gifInfo); err != nil {
		logrus.WithField("error", err).Error("While encoding gif file")
		return
	}
}
