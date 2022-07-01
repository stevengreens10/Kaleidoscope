package parser

type Type int8

const (
	Invalid Type = -1
	Double       = iota
	String       = iota
	Void         = iota
)
