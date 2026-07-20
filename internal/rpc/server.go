// Package rpc is the gRPC transport for dropcrate. It adapts streaming gRPC
// calls onto the same service.Service the HTTP API uses, so both transports
// share one set of upload/download/delete use cases.
package rpc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/IgliHoxha/dropcrate/internal/files"
	pb "github.com/IgliHoxha/dropcrate/internal/rpc/dropcratev1"
	"github.com/IgliHoxha/dropcrate/internal/service"
)

// downloadChunkSize is the payload size of each Download stream message.
const downloadChunkSize = 64 * 1024

// Sentinel errors used to abort the Upload pipe; the atomic flags they set are
// what the handler actually checks, since the store may wrap the pipe error.
var (
	errUploadTooLarge = errors.New("upload too large")
	errRepeatedInfo   = errors.New("info sent more than once")
)

// Server implements the generated FileServiceServer over a service.Service.
type Server struct {
	pb.UnimplementedFileServiceServer
	svc       *service.Service
	log       *slog.Logger
	maxUpload int64
}

// NewServer constructs a gRPC FileService backed by svc. The upload size limit
// is taken from the service so both transports share one source of truth.
func NewServer(svc *service.Service, log *slog.Logger) *Server {
	return &Server{svc: svc, log: log, maxUpload: svc.MaxUpload()}
}

// Upload consumes a client stream whose first message carries the file info and
// whose remaining messages carry the bytes. The bytes are piped straight into
// object storage (size unknown, -1) rather than staged on local disk, and the
// size limit is enforced on the stream before bytes are forwarded.
func (s *Server) Upload(stream pb.FileService_UploadServer) error {
	first, err := stream.Recv()
	if err != nil {
		return status.Error(codes.InvalidArgument, "expected an upload info message")
	}
	info := first.GetInfo()
	if info == nil {
		return status.Error(codes.InvalidArgument, "first message must carry 'info'")
	}

	pr, pw := io.Pipe()
	// These record why the feeder stopped so the outcome can be mapped to the
	// right status code regardless of how the store wraps the pipe error.
	var tooLarge, repeatedInfo atomic.Bool

	go func() {
		var written int64
		for {
			msg, recvErr := stream.Recv()
			if recvErr == io.EOF {
				_ = pw.Close()
				return
			}
			if recvErr != nil {
				_ = pw.CloseWithError(recvErr)
				return
			}
			if msg.GetInfo() != nil {
				repeatedInfo.Store(true)
				_ = pw.CloseWithError(errRepeatedInfo)
				return
			}
			chunk := msg.GetChunk()
			if s.maxUpload > 0 && written+int64(len(chunk)) > s.maxUpload {
				tooLarge.Store(true)
				_ = pw.CloseWithError(errUploadTooLarge)
				return
			}
			if _, wErr := pw.Write(chunk); wErr != nil {
				return // reader closed (store failed); nothing more to do
			}
			written += int64(len(chunk))
		}
	}()

	f, err := s.svc.Upload(stream.Context(), service.UploadInput{
		Filename:    info.GetFilename(),
		ContentType: info.GetContentType(),
		Size:        -1, // unknown; streamed
		Body:        pr,
		TTL:         ttlFromSeconds(info.GetTtlSeconds()),
	})
	if err != nil {
		// Unblock the feeder if it is still writing.
		_ = pr.CloseWithError(err)
		switch {
		case tooLarge.Load() || errors.Is(err, service.ErrTooLarge):
			return status.Errorf(codes.ResourceExhausted, "upload exceeds the %d byte limit", s.maxUpload)
		case repeatedInfo.Load():
			return status.Error(codes.InvalidArgument, "info may only be sent once, as the first message")
		default:
			s.log.Error("upload failed", "error", err)
			return status.Error(codes.Internal, "could not store file")
		}
	}

	return stream.SendAndClose(toFileInfo(f))
}

// Download streams a file's metadata (first message) followed by its bytes.
func (s *Server) Download(req *pb.DownloadRequest, stream pb.FileService_DownloadServer) error {
	f, obj, err := s.svc.Download(stream.Context(), req.GetId())
	if err != nil {
		return lookupStatus(err, "download", s.log)
	}
	defer obj.Body.Close()

	if err := stream.Send(&pb.DownloadResponse{
		Payload: &pb.DownloadResponse_Info{Info: toFileInfo(f)},
	}); err != nil {
		return err
	}

	buf := make([]byte, downloadChunkSize)
	for {
		n, readErr := obj.Body.Read(buf)
		if n > 0 {
			if sErr := stream.Send(&pb.DownloadResponse{
				Payload: &pb.DownloadResponse_Chunk{Chunk: buf[:n]},
			}); sErr != nil {
				return sErr
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			s.log.Error("stream failed", "id", req.GetId(), "error", readErr)
			return status.Error(codes.Internal, "could not read file")
		}
	}
}

// GetMetadata returns a file's record without its bytes.
func (s *Server) GetMetadata(ctx context.Context, req *pb.GetMetadataRequest) (*pb.FileInfo, error) {
	f, err := s.svc.Metadata(ctx, req.GetId())
	if err != nil {
		return nil, lookupStatus(err, "metadata", s.log)
	}
	return toFileInfo(f), nil
}

// Delete removes a file. Deleting an unknown id succeeds.
func (s *Server) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	if err := s.svc.Delete(ctx, req.GetId()); err != nil {
		s.log.Error("delete failed", "error", err)
		return nil, status.Error(codes.Internal, "could not delete file")
	}
	return &pb.DeleteResponse{}, nil
}

// toFileInfo maps the domain model onto the wire type.
func toFileInfo(f files.File) *pb.FileInfo {
	out := &pb.FileInfo{
		Id:          f.ID,
		Filename:    f.Filename,
		ContentType: f.ContentType,
		Size:        f.Size,
		CreatedAt:   timestamppb.New(f.CreatedAt),
	}
	if f.ExpiresAt != nil {
		out.ExpiresAt = timestamppb.New(*f.ExpiresAt)
	}
	return out
}

// ttlFromSeconds maps the proto TTL convention onto service.UploadInput.TTL:
// 0 -> server default, negative -> never expire, positive -> that many seconds.
func ttlFromSeconds(secs int64) time.Duration {
	switch {
	case secs < 0:
		return -1
	case secs > 0:
		return time.Duration(secs) * time.Second
	default:
		return 0
	}
}

// lookupStatus maps a service lookup error onto a gRPC status, treating a
// missing file as NotFound and logging anything unexpected.
func lookupStatus(err error, op string, log *slog.Logger) error {
	if errors.Is(err, files.ErrNotFound) {
		return status.Error(codes.NotFound, "file not found")
	}
	log.Error(op+" failed", "error", err)
	return status.Error(codes.Internal, "internal error")
}
