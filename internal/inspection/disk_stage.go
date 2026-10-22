package inspection

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"os"

	"golang.org/x/sys/unix"
)

var (
	errDiskStageLimit        = errors.New("expanded disk member exceeds staging limit")
	errDiskStageSpace        = errors.New("insufficient free space for disk staging reserve")
	errDiskStageSizeMismatch = errors.New("expanded disk member size differs from ZIP declaration")
)

const (
	diskStageLimit   = 8 << 30
	diskStageReserve = 2 << 30
)

type diskStageOptions struct {
	root    string
	limit   uint64
	reserve uint64
	free    func(string) (uint64, error)
}

func defaultDiskStageOptions() diskStageOptions {
	return diskStageOptions{
		root:    os.TempDir(),
		limit:   diskStageLimit,
		reserve: diskStageReserve,
		free:    diskStageFree,
	}
}

func diskStageFree(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("disk staging free space: %w", err)
	}
	hi, lo := bits.Mul64(uint64(stat.Bavail), uint64(stat.Bsize))
	if hi != 0 {
		return ^uint64(0), nil
	}
	return lo, nil
}

type stagedDisk struct {
	file *os.File
	dir  string
	size uint64
}

func (s *stagedDisk) Close() error {
	return errors.Join(s.file.Close(), os.RemoveAll(s.dir))
}

type diskStageWriter struct {
	ctx     context.Context
	file    *os.File
	limit   uint64
	reserve uint64
	written uint64
	free    func(string) (uint64, error)
}

func (w *diskStageWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, fmt.Errorf("disk staging cancelled: %w", err)
	}
	if uint64(len(p)) > w.limit-w.written {
		return 0, fmt.Errorf("%w: measured expansion exceeds %d bytes", errDiskStageLimit, w.limit)
	}
	free, err := w.free(w.file.Name())
	if err != nil {
		return 0, err
	}
	if free < w.reserve || uint64(len(p)) > free-w.reserve {
		return 0, fmt.Errorf(
			"%w: need %d bytes plus %d-byte reserve, have %d bytes",
			errDiskStageSpace,
			len(p),
			w.reserve,
			free,
		)
	}
	n, err := w.file.Write(p)
	w.written += uint64(n) //nolint:gosec // os.File.Write returns a nonnegative byte count.
	return n, err          //nolint:wrapcheck // io.Writer propagates short writes and filesystem errors.
}

func stageDiskMember(
	ctx context.Context,
	member *zip.File,
	opts diskStageOptions,
) (_ *stagedDisk, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("disk staging cancelled: %w", err)
	}
	if member.UncompressedSize64 > opts.limit {
		return nil, fmt.Errorf(
			"%w: declared %d bytes exceeds %d bytes",
			errDiskStageLimit,
			member.UncompressedSize64,
			opts.limit,
		)
	}
	free, err := opts.free(opts.root)
	if err != nil {
		return nil, err
	}
	if free < opts.reserve || member.UncompressedSize64 > free-opts.reserve {
		return nil, fmt.Errorf(
			"%w: declared %d bytes plus %d-byte reserve, have %d bytes",
			errDiskStageSpace,
			member.UncompressedSize64,
			opts.reserve,
			free,
		)
	}
	dir, err := os.MkdirTemp(opts.root, "go-ios-vm-disk-")
	if err != nil {
		return nil, fmt.Errorf("disk staging directory: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(dir))
		}
	}()
	file, err := os.CreateTemp(dir, "image-*.partial")
	if err != nil {
		return nil, fmt.Errorf("disk staging file: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()
	r, err := member.Open()
	if err != nil {
		return nil, fmt.Errorf("open compressed disk member: %w", err)
	}
	defer r.Close()
	writer := &diskStageWriter{
		ctx:     ctx,
		file:    file,
		limit:   opts.limit,
		reserve: opts.reserve,
		free:    opts.free,
	}
	_, err = io.CopyBuffer(writer, contextReader{ctx, r}, make([]byte, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("copy compressed disk member: %w", err)
	}
	if writer.written != member.UncompressedSize64 {
		return nil, fmt.Errorf(
			"%w: declared %d bytes, measured %d bytes",
			errDiskStageSizeMismatch,
			member.UncompressedSize64,
			writer.written,
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("disk staging cancelled: %w", err)
	}
	return &stagedDisk{file: file, dir: dir, size: writer.written}, nil
}
