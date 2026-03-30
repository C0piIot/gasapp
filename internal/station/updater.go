package station

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const pricesURL = "https://sedeaplicaciones.minetur.gob.es/ServiciosRESTCarburantes/PreciosCarburantes/EstacionesTerrestres/"

// UpdatePrices fetches the government XML feed and upserts all stations with prices.
// curl is used instead of Go's HTTP client because the MINETUR server rejects
// Go's TLS ClientHello fingerprint regardless of HTTP version or User-Agent.
func UpdatePrices(db *sql.DB) error {
	cmd := exec.Command("curl", "--http1.1", "--silent", "--fail",
		"--header", "Accept: application/xml",
		"--user-agent", "Mozilla/5.0",
		pricesURL)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	parseErr := parseStream(db, stdout)

	if waitErr := cmd.Wait(); waitErr != nil && parseErr == nil {
		return waitErr
	}
	return parseErr
}

// parseStream iteratively parses the XML response, upserting one station at a time
// to keep memory usage flat regardless of feed size.
func parseStream(db *sql.DB, r io.Reader) error {
	dec := xml.NewDecoder(r)
	var fields map[string]string
	var inStation bool

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("xml token: %w", err)
		}

		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == "EESSPrecio" {
				fields = make(map[string]string, 20)
				inStation = true
			} else if inStation {
				var text string
				if err := dec.DecodeElement(&text, &se); err != nil {
					return fmt.Errorf("xml decode field %s: %w", se.Name.Local, err)
				}
				fields[se.Name.Local] = strings.TrimSpace(text)
			}
		case xml.EndElement:
			if se.Name.Local == "EESSPrecio" && inStation {
				if s, ok := buildStation(fields); ok {
					if err := Upsert(db, s); err != nil {
						return fmt.Errorf("upsert station %s: %w", fields["IDEESS"], err)
					}
				}
				inStation = false
			}
		}
	}
	return nil
}

func buildStation(f map[string]string) (Station, bool) {
	petrol95 := firstNonEmpty(
		f["Precio_x0020_Gasolina_x0020_95_x0020_E5"],
		f["Precio_x0020_Gasolina_x0020_95_x0020_E5_x0020_Premium"],
	)
	petrol98 := firstNonEmpty(
		f["Precio_x0020_Gasolina_x0020_98_x0020_E5"],
		f["Precio_x0020_Gasolina_x0020_98_x0020_E10"],
	)
	gasoil := firstNonEmpty(
		f["Precio_x0020_Gasoleo_x0020_A"],
		f["Precio_x0020_Gasoleo_x0020_Premium"],
	)
	glp := f["Precio_x0020_Gases_x0020_licuados_x0020_del_x0020_petróleo"]

	if petrol95 == "" && petrol98 == "" && gasoil == "" && glp == "" {
		return Station{}, false
	}

	id, err := strconv.ParseInt(f["IDEESS"], 10, 64)
	if err != nil {
		return Station{}, false
	}
	lat, err := spanishFloat(f["Latitud"])
	if err != nil {
		return Station{}, false
	}
	lng, err := spanishFloat(f["Longitud_x0020__x0028_WGS84_x0029_"])
	if err != nil {
		return Station{}, false
	}

	return Station{
		ID:           id,
		Name:         titleCase(f["Rótulo"]),
		Updated:      time.Now().Unix(),
		PostalCode:   f["C.P."],
		Address:      titleCase(f["Dirección"]),
		OpeningHours: f["Horario"],
		Town:         titleCase(f["Localidad"]),
		City:         f["Municipio"],
		State:        titleCase(f["Provincia"]),
		Lat:          lat,
		Lng:          lng,
		Petrol95:     optFloat(petrol95),
		Petrol98:     optFloat(petrol98),
		Gasoil:       optFloat(gasoil),
		GLP:          optFloat(glp),
	}, true
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func spanishFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
}

func optFloat(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := spanishFloat(s)
	if err != nil {
		return nil
	}
	return &f
}

// titleCase mirrors Python's str.title(): capitalises the first letter of each word.
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		runes := []rune(w)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
		}
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
