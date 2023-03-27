package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	cfgLevel    = os.Getenv("LEVEL")
	cfgListenOn = os.Getenv("LISTEN")
)

var saveImage = func(fname string, m image.Image) error {
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	return png.Encode(f, m)
}

const shutdownTimeout = 30 * time.Second

func main() {
	if level, e := zerolog.ParseLevel(cfgLevel); e == nil {
		zerolog.SetGlobalLevel(level)
	} else {
		fmt.Fprintf(os.Stderr, "unable to parse level %s", cfgLevel)
	}

	echoServer := echo.New()
	echoServer.Use(middleware.CORS())
	echoServer.Debug = true
	echoServer.POST("/rembg", rembg)

	doMain(func(ctx context.Context, cancel context.CancelFunc) error {
		defer cancel()

		go func() {
			defer cancel()
			// echo server start

			if cfgListenOn == "" {
				log.Error().Msg("Specify LISTEN env")
				return
			}

			if e := echoServer.Start(cfgListenOn); e != nil && !errors.Is(e, http.ErrServerClosed) {
				log.Error().Err(e).Msg("Unable to start echo server")
			}
		}()

		<-ctx.Done()

		// gracefully shutdown our pretty HTTP server
		closeCtx, closeCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer closeCancel()

		return echoServer.Shutdown(closeCtx) //nolint:contextcheck
	})
}

// doMain starts function runFunc with context. The context will be canceled
// by SIGTERM or SIGINT signal (Ctrl+C for example)
// beforeExit function must be executed immediately before exit
func doMain(runFunc func(ctx context.Context, cancel context.CancelFunc) error) {
	// context should be canceled while Int signal will be caught
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// main processing loop
	retChan := make(chan error, 1)
	go func() {
		err2 := runFunc(ctx, cancel)
		if err2 != nil {
			retChan <- err2
		}
		close(retChan)
	}()

	// Waiting signals from OS
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
		log.Warn().Msgf("Signal '%s' was caught. Exiting", <-quit)
		cancel()
	}()

	// Listening for the main loop response
	for e := range retChan {
		log.Error().Err(e).Msg("Exiting.")
	}
}

const (
	rembgPath = "/usr/local/bin/rembg"
)

// rembgExec removes the background from the image
func rembgExec(ctx context.Context, src image.Image) (image.Image, error) {
	srcBuffer := &bytes.Buffer{}
	dstBuffer := &bytes.Buffer{}
	errBuffer := &bytes.Buffer{}

	err := png.Encode(srcBuffer, src)
	if err != nil {
		return nil, fmt.Errorf("unable to encode source image to PNG: %w", err)
	}

	cmd := exec.CommandContext(ctx, rembgPath, "i")

	cmd.Stdin = srcBuffer
	cmd.Stdout = dstBuffer
	cmd.Stderr = errBuffer

	err = cmd.Run()
	if err != nil {
		log.Error().Err(err).Str("stderr", errBuffer.String()).Str("cmd", cmd.String()).Msg("Error while rembg")
		return nil, fmt.Errorf("failed to remove background with rembg: %w", err)
	}

	dst, _, err := image.Decode(dstBuffer)
	if err != nil {
		return nil, fmt.Errorf("failed to decode answer from rembg: %w", err)
	}

	return dst, nil
}

func rembg(c echo.Context) error {
	ctx := c.Request().Context()

	src, _, err := image.Decode(c.Request().Body)
	if err != nil {
		return err
	}

	woBG, err := rembgExec(ctx, src)
	if err != nil {
		return errors.Wrap(err, "unable to exec rembg")
	}

	c.Response().Status = http.StatusOK
	c.Response().Header().Set(echo.HeaderContentType, "image/png")
	return png.Encode(c.Response(), woBG)
}
