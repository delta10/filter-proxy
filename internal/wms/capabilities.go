package wms

import "encoding/xml"

type Capabilities struct {
	XMLName        xml.Name `xml:"WMS_Capabilities"`
	Text           string   `xml:",chardata"`
	Version        string   `xml:"version,attr"`
	UpdateSequence string   `xml:"updateSequence,attr"`
	Xmlns          string   `xml:"xmlns,attr"`
	Xlink          string   `xml:"xlink,attr"`
	Xsi            string   `xml:"xsi,attr"`
	SchemaLocation string   `xml:"schemaLocation,attr"`
	Service        struct {
		Text        string `xml:",chardata"`
		Name        string `xml:"Name"`
		Title       string `xml:"Title"`
		Abstract    string `xml:"Abstract"`
		KeywordList struct {
			Text    string   `xml:",chardata"`
			Keyword []string `xml:"Keyword"`
		} `xml:"KeywordList"`
		OnlineResource struct {
			Text string `xml:",chardata"`
			Type string `xml:"type,attr"`
			Href string `xml:"href,attr"`
		} `xml:"OnlineResource"`
		ContactInformation struct {
			Text                 string `xml:",chardata"`
			ContactPersonPrimary struct {
				Text                string `xml:",chardata"`
				ContactPerson       string `xml:"ContactPerson"`
				ContactOrganization string `xml:"ContactOrganization"`
			} `xml:"ContactPersonPrimary"`
			ContactPosition string `xml:"ContactPosition"`
			ContactAddress  struct {
				Text            string `xml:",chardata"`
				AddressType     string `xml:"AddressType"`
				Address         string `xml:"Address"`
				City            string `xml:"City"`
				StateOrProvince string `xml:"StateOrProvince"`
				PostCode        string `xml:"PostCode"`
				Country         string `xml:"Country"`
			} `xml:"ContactAddress"`
			ContactVoiceTelephone        string `xml:"ContactVoiceTelephone"`
			ContactFacsimileTelephone    string `xml:"ContactFacsimileTelephone"`
			ContactElectronicMailAddress string `xml:"ContactElectronicMailAddress"`
		} `xml:"ContactInformation"`
		Fees              string `xml:"Fees"`
		AccessConstraints string `xml:"AccessConstraints"`
	} `xml:"Service"`
	Capability struct {
		Text    string `xml:",chardata"`
		Request struct {
			Text            string `xml:",chardata"`
			GetCapabilities struct {
				Text    string `xml:",chardata"`
				Format  string `xml:"Format"`
				DCPType struct {
					Text string `xml:",chardata"`
					HTTP struct {
						Text string `xml:",chardata"`
						Get  struct {
							Text           string `xml:",chardata"`
							OnlineResource struct {
								Text string `xml:",chardata"`
								Type string `xml:"type,attr"`
								Href string `xml:"href,attr"`
							} `xml:"OnlineResource"`
						} `xml:"Get"`
						Post struct {
							Text           string `xml:",chardata"`
							OnlineResource struct {
								Text string `xml:",chardata"`
								Type string `xml:"type,attr"`
								Href string `xml:"href,attr"`
							} `xml:"OnlineResource"`
						} `xml:"Post"`
					} `xml:"HTTP"`
				} `xml:"DCPType"`
			} `xml:"GetCapabilities"`
			GetMap struct {
				Text    string   `xml:",chardata"`
				Format  []string `xml:"Format"`
				DCPType struct {
					Text string `xml:",chardata"`
					HTTP struct {
						Text string `xml:",chardata"`
						Get  struct {
							Text           string `xml:",chardata"`
							OnlineResource struct {
								Text string `xml:",chardata"`
								Type string `xml:"type,attr"`
								Href string `xml:"href,attr"`
							} `xml:"OnlineResource"`
						} `xml:"Get"`
					} `xml:"HTTP"`
				} `xml:"DCPType"`
			} `xml:"GetMap"`
			GetFeatureInfo struct {
				Text    string   `xml:",chardata"`
				Format  []string `xml:"Format"`
				DCPType struct {
					Text string `xml:",chardata"`
					HTTP struct {
						Text string `xml:",chardata"`
						Get  struct {
							Text           string `xml:",chardata"`
							OnlineResource struct {
								Text string `xml:",chardata"`
								Type string `xml:"type,attr"`
								Href string `xml:"href,attr"`
							} `xml:"OnlineResource"`
						} `xml:"Get"`
					} `xml:"HTTP"`
				} `xml:"DCPType"`
			} `xml:"GetFeatureInfo"`
		} `xml:"Request"`
		Exception struct {
			Text   string   `xml:",chardata"`
			Format []string `xml:"Format"`
		} `xml:"Exception"`
		Layer struct {
			Text                    string   `xml:",chardata"`
			Title                   string   `xml:"Title"`
			Abstract                string   `xml:"Abstract"`
			CRS                     []string `xml:"CRS"`
			EXGeographicBoundingBox struct {
				Text               string `xml:",chardata"`
				WestBoundLongitude string `xml:"westBoundLongitude"`
				EastBoundLongitude string `xml:"eastBoundLongitude"`
				SouthBoundLatitude string `xml:"southBoundLatitude"`
				NorthBoundLatitude string `xml:"northBoundLatitude"`
			} `xml:"EX_GeographicBoundingBox"`
			BoundingBox struct {
				Text string `xml:",chardata"`
				CRS  string `xml:"CRS,attr"`
				Minx string `xml:"minx,attr"`
				Miny string `xml:"miny,attr"`
				Maxx string `xml:"maxx,attr"`
				Maxy string `xml:"maxy,attr"`
			} `xml:"BoundingBox"`
			Layer []struct {
				Text        string `xml:",chardata"`
				Queryable   string `xml:"queryable,attr"`
				Opaque      string `xml:"opaque,attr"`
				Cascaded    string `xml:"cascaded,attr"`
				Name        string `xml:"Name"`
				Title       string `xml:"Title"`
				Abstract    string `xml:"Abstract"`
				KeywordList struct {
					Text    string   `xml:",chardata"`
					Keyword []string `xml:"Keyword"`
				} `xml:"KeywordList"`
				CRS                     []string `xml:"CRS"`
				EXGeographicBoundingBox struct {
					Text               string `xml:",chardata"`
					WestBoundLongitude string `xml:"westBoundLongitude"`
					EastBoundLongitude string `xml:"eastBoundLongitude"`
					SouthBoundLatitude string `xml:"southBoundLatitude"`
					NorthBoundLatitude string `xml:"northBoundLatitude"`
				} `xml:"EX_GeographicBoundingBox"`
				BoundingBox []struct {
					Text string `xml:",chardata"`
					CRS  string `xml:"CRS,attr"`
					Minx string `xml:"minx,attr"`
					Miny string `xml:"miny,attr"`
					Maxx string `xml:"maxx,attr"`
					Maxy string `xml:"maxy,attr"`
				} `xml:"BoundingBox"`
				Style []struct {
					Text      string `xml:",chardata"`
					Name      string `xml:"Name"`
					Title     string `xml:"Title"`
					Abstract  string `xml:"Abstract"`
					LegendURL struct {
						Text           string `xml:",chardata"`
						Width          string `xml:"width,attr"`
						Height         string `xml:"height,attr"`
						Format         string `xml:"Format"`
						OnlineResource struct {
							Text  string `xml:",chardata"`
							Xlink string `xml:"xlink,attr"`
							Type  string `xml:"type,attr"`
							Href  string `xml:"href,attr"`
						} `xml:"OnlineResource"`
					} `xml:"LegendURL"`
				} `xml:"Style"`
				MaxScaleDenominator string `xml:"MaxScaleDenominator"`
				MinScaleDenominator string `xml:"MinScaleDenominator"`
				Dimension           struct {
					Text    string `xml:",chardata"`
					Name    string `xml:"name,attr"`
					Default string `xml:"default,attr"`
					Units   string `xml:"units,attr"`
				} `xml:"Dimension"`
			} `xml:"Layer"`
		} `xml:"Layer"`
	} `xml:"Capability"`
}
