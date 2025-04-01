package wfs

import "encoding/xml"

type Transaction struct {
	XMLName        xml.Name       `xml:"http://www.opengis.net/wfs Transaction"`
	Service        string         `xml:"service,attr"`
	Version        string         `xml:"version,attr"`
	XMLNS          string         `xml:"xmlns,attr"`
	XSI            string         `xml:"xmlns:xsi,attr"`
	SchemaLocation string         `xml:"xsi:schemaLocation,attr"`
	Inserts        []Action       `xml:"Insert"`
	Updates        []UpdateAction `xml:"Update"`
	Deletes        []DeleteAction `xml:"Delete"`
}

type Action struct {
	Layers []Layer `xml:",any"`
}

type Layer struct {
	XMLName xml.Name
	Attrs   []xml.Attr  `xml:",any,attr"`
	Content []InnerNode `xml:",any"`
}

type InnerNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr  `xml:",any,attr"`
	Content string      `xml:",chardata"`
	Nodes   []InnerNode `xml:",any"`
}

type UpdateAction struct {
	XMLName  xml.Name   `xml:"Update"`
	TypeName string     `xml:"typeName,attr"`
	Props    []Property `xml:"Property"`
	Filter   *Filter    `xml:"http://www.opengis.net/ogc Filter"`
}

type Property struct {
	Name  string    `xml:"Name"`
	Value *XMLValue `xml:"Value"`
}

type XMLValue struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Content string     `xml:",chardata"`
	Nodes   []XMLValue `xml:",any"`
}

type DeleteAction struct {
	XMLName  xml.Name `xml:"Delete"`
	TypeName string   `xml:"typeName,attr"`

	Filter *Filter `xml:"http://www.opengis.net/ogc Filter"`
}

type Filter struct {
	FeatureID FeatureID `xml:"FeatureId"`
}

type FeatureID struct {
	FID string `xml:"fid,attr"`
}
