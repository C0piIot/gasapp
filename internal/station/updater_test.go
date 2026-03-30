package station

import (
	"path/filepath"
	"strings"
	"testing"

	"gasapp/internal/db"
)

func TestTitleCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"REPSOL", "Repsol"},
		{"calle mayor", "Calle Mayor"},
		{"JOSÉ MARÍA", "José María"},
		{"", ""},
	}
	for _, c := range cases {
		if got := titleCase(c.in); got != c.want {
			t.Errorf("titleCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSpanishFloat(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"1,659", 1.659, false},
		{"-3,7038", -3.7038, false},
		{"40,4168", 40.4168, false},
		{"1.659", 1.659, false}, // dot already — passes through
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, c := range cases {
		got, err := spanishFloat(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("spanishFloat(%q) error=%v, wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("spanishFloat(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"a", "b", "a"},
		{"", "b", "b"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := firstNonEmpty(c.a, c.b); got != c.want {
			t.Errorf("firstNonEmpty(%q,%q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

// baseFields returns a minimal valid field map with no prices set.
func baseFields() map[string]string {
	return map[string]string{
		"IDEESS":    "1234",
		"Rótulo":    "REPSOL",
		"C.P.":      "28001",
		"Dirección": "CALLE MAYOR 1",
		"Horario":   "L-D: 24H",
		"Localidad": "MADRID",
		"Municipio": "MADRID",
		"Provincia": "MADRID",
		"Latitud":   "40,4168",
		"Longitud_x0020__x0028_WGS84_x0029_": "-3,7038",
	}
}

func TestBuildStation(t *testing.T) {
	t.Run("standard prices", func(t *testing.T) {
		f := baseFields()
		f["Precio_x0020_Gasolina_x0020_95_x0020_E5"] = "1,659"
		f["Precio_x0020_Gasolina_x0020_98_x0020_E5"] = "1,799"
		f["Precio_x0020_Gasoleo_x0020_A"] = "1,559"

		s, ok := buildStation(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if s.ID != 1234 {
			t.Errorf("ID = %d, want 1234", s.ID)
		}
		if s.Name != "Repsol" {
			t.Errorf("Name = %q, want Repsol", s.Name)
		}
		if s.Petrol95 == nil || *s.Petrol95 != 1.659 {
			t.Errorf("Petrol95 = %v, want 1.659", s.Petrol95)
		}
		if s.Petrol98 == nil || *s.Petrol98 != 1.799 {
			t.Errorf("Petrol98 = %v, want 1.799", s.Petrol98)
		}
		if s.Gasoil == nil || *s.Gasoil != 1.559 {
			t.Errorf("Gasoil = %v, want 1.559", s.Gasoil)
		}
		if s.GLP != nil {
			t.Errorf("GLP = %v, want nil", s.GLP)
		}
		if s.Lat != 40.4168 {
			t.Errorf("Lat = %v, want 40.4168", s.Lat)
		}
		if s.Lng != -3.7038 {
			t.Errorf("Lng = %v, want -3.7038", s.Lng)
		}
	})

	t.Run("falls back to alternate petrol95 field", func(t *testing.T) {
		f := baseFields()
		f["Precio_x0020_Gasolina_x0020_95_x0020_E5_x0020_Premium"] = "1,699"

		s, ok := buildStation(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if s.Petrol95 == nil || *s.Petrol95 != 1.699 {
			t.Errorf("Petrol95 = %v, want 1.699", s.Petrol95)
		}
	})

	t.Run("falls back to alternate petrol98 field", func(t *testing.T) {
		f := baseFields()
		f["Precio_x0020_Gasolina_x0020_98_x0020_E10"] = "1,849"

		s, ok := buildStation(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if s.Petrol98 == nil || *s.Petrol98 != 1.849 {
			t.Errorf("Petrol98 = %v, want 1.849", s.Petrol98)
		}
	})

	t.Run("falls back to alternate gasoil field", func(t *testing.T) {
		f := baseFields()
		f["Precio_x0020_Gasoleo_x0020_Premium"] = "1,659"

		s, ok := buildStation(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if s.Gasoil == nil || *s.Gasoil != 1.659 {
			t.Errorf("Gasoil = %v, want 1.659", s.Gasoil)
		}
	})

	t.Run("glp only", func(t *testing.T) {
		f := baseFields()
		f["Precio_x0020_Gases_x0020_licuados_x0020_del_x0020_petróleo"] = "0,859"

		s, ok := buildStation(f)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if s.GLP == nil || *s.GLP != 0.859 {
			t.Errorf("GLP = %v, want 0.859", s.GLP)
		}
		if s.Petrol95 != nil || s.Petrol98 != nil || s.Gasoil != nil {
			t.Error("expected nil for petrol95/petrol98/gasoil")
		}
	})

	t.Run("no prices skipped", func(t *testing.T) {
		_, ok := buildStation(baseFields())
		if ok {
			t.Error("expected ok=false for station with no prices")
		}
	})
}

func TestParseStream(t *testing.T) {
	// Station 1001: valid with prices.
	// Station 1002: no prices — must be skipped.
	const xmlData = `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="http://schemas.datacontract.org/2004/07/ServiciosCarburantes">
  <EESSPrecio>
    <IDEESS>1001</IDEESS>
    <Rótulo>REPSOL</Rótulo>
    <C.P.>28001</C.P.>
    <Dirección>CALLE MAYOR 1</Dirección>
    <Horario>L-D: 24H</Horario>
    <Localidad>MADRID</Localidad>
    <Municipio>MADRID</Municipio>
    <Provincia>MADRID</Provincia>
    <Precio_x0020_Gasolina_x0020_95_x0020_E5>1,659</Precio_x0020_Gasolina_x0020_95_x0020_E5>
    <Precio_x0020_Gasolina_x0020_98_x0020_E5>1,799</Precio_x0020_Gasolina_x0020_98_x0020_E5>
    <Precio_x0020_Gasoleo_x0020_A>1,559</Precio_x0020_Gasoleo_x0020_A>
    <Latitud>40,4168</Latitud>
    <Longitud_x0020__x0028_WGS84_x0029_>-3,7038</Longitud_x0020__x0028_WGS84_x0029_>
  </EESSPrecio>
  <EESSPrecio>
    <IDEESS>1002</IDEESS>
    <Rótulo>NO PRICES</Rótulo>
    <C.P.>28002</C.P.>
    <Dirección>CALLE SIN PRECIO 2</Dirección>
    <Horario>L-D: 24H</Horario>
    <Localidad>MADRID</Localidad>
    <Municipio>MADRID</Municipio>
    <Provincia>MADRID</Provincia>
    <Latitud>40,5000</Latitud>
    <Longitud_x0020__x0028_WGS84_x0029_>-3,7000</Longitud_x0020__x0028_WGS84_x0029_>
  </EESSPrecio>
</root>`

	database, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := parseStream(database, strings.NewReader(xmlData)); err != nil {
		t.Fatal(err)
	}

	stations, err := All(database)
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 1 {
		t.Fatalf("got %d stations, want 1 (no-price station must be skipped)", len(stations))
	}
	if stations[0].ID != 1001 {
		t.Errorf("station ID = %d, want 1001", stations[0].ID)
	}
	if stations[0].Petrol95 == nil || *stations[0].Petrol95 != 1.659 {
		t.Errorf("Petrol95 = %v, want 1.659", stations[0].Petrol95)
	}
}
