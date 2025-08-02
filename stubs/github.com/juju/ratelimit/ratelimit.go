package ratelimit

import "io"

type Bucket struct{}

func NewBucketWithRate(rate float64, capacity int64) *Bucket { return &Bucket{} }

type writer struct{ io.Writer }

func (w writer) Write(p []byte) (int, error) { return w.Writer.Write(p) }

func Writer(w io.Writer, b *Bucket) io.Writer { return writer{w} }
