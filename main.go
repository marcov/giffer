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
	MYNAME  = "giffer"
	VERSION = "1.0"
	OUTFILE = "output.gif"
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

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `NAME:
   %s - generate animated gifs from jpeg files

USAGE:
   %s [options] <path>

By default, %s searches for jpeg files at the specified path and writes the animated gif to %s

Options:
`, MYNAME, MYNAME, MYNAME, OUTFILE)

	flag.PrintDefaults()
}

func main() {
	verbose := flag.Bool("d", false, "debug mode")
	outfile := flag.String("o", OUTFILE, "write the animated git to this destination")
	delayMs := flag.Uint("t", 100, "gif inter-frame delay (ms)")
	version := flag.Bool("v", false, "print version and exit")

	flag.Usage = usage
	flag.Parse()

	if *verbose {
		fmt.Println("Starting in debug mode...")
		logrus.SetLevel(logrus.DebugLevel)
	}

	if *version {
		fmt.Printf("%s -- v%s\n", MYNAME, VERSION)
		return
	}

	_, err := os.Stat(*outfile)
	if !os.IsNotExist(err) {
		logrus.WithFields(logrus.Fields{"file": *outfile}).Error("output file already exists")
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		usage()
		return
	}

	if len(args) > 1 {
		logrus.Error("wrong number of arguments")
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

	if len(imgPaths) == 0 {
		logrus.Errorf("could not find any jpeg files at provided path")
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
		"// jobs":     numcpus,
		"num of pics": len(imgPaths),
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
