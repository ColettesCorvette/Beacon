package gather

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTempMilli(t *testing.T) {
	if v, ok := parseTempMilli("45000\n"); !ok || v != 45.0 {
		t.Errorf("parseTempMilli(45000) = %v,%v want 45,true", v, ok)
	}
	if _, ok := parseTempMilli("abc"); ok {
		t.Errorf("parseTempMilli(abc) should fail")
	}
}

func TestParseThrottleCount(t *testing.T) {
	line := "6,123,456789,-;mce: CPU0: Core temperature above threshold, cpu clock throttled (total events = 12)"
	if v, ok := parseThrottleCount(line); !ok || v != 12 {
		t.Errorf("parseThrottleCount = %v,%v want 12,true", v, ok)
	}
	if _, ok := parseThrottleCount("6,1,1,-;random kernel message"); ok {
		t.Errorf("parseThrottleCount should fail on unrelated line")
	}
}

func TestReadThrottleCount(t *testing.T) {
	kmsg := "6,1,1,-;Core temperature above threshold, cpu clock throttled (total events = 3)\n" +
		"6,2,2,-;Core temperature/speed normal\n" +
		"6,3,3,-;Core temperature above threshold, cpu clock throttled (total events = 7)\n"
	v, ok := readThrottleCount(strings.NewReader(kmsg))
	if !ok || v != 7 {
		t.Errorf("readThrottleCount = %v,%v want 7,true (dernier compteur)", v, ok)
	}
}

func TestReadSensorsFromDir_Hwmon(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "hwmon0")
	if err := os.MkdirAll(hwmon, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(hwmon, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("name", "coretemp\n")
	write("temp1_input", "45000\n")
	write("temp1_crit", "100000\n")
	write("temp1_label", "Core 0\n")

	got := readSensorsFromDir(root, filepath.Join(root, "nonexistent-thermal"))
	if len(got) != 1 {
		t.Fatalf("expected 1 reading, got %d (%+v)", len(got), got)
	}
	r := got[0]
	if r.Sensor != "coretemp" || r.Label != "Core 0" || r.Temp != 45.0 || !r.HasMax || r.Max != 100.0 {
		t.Errorf("reading = %+v, want coretemp/Core 0/45/100", r)
	}
}

func TestReadHwmon_FiltersNonCPU(t *testing.T) {
	root := t.TempDir()
	mk := func(dir string, files map[string]string) {
		d := filepath.Join(root, dir)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(d, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Reproduit la machine de test : coretemp (CPU) + capteurs non-CPU à écarter.
	mk("hwmon0", map[string]string{"name": "acpitz\n", "temp1_input": "27800\n"})
	mk("hwmon1", map[string]string{"name": "pch_cannonlake\n", "temp1_input": "43000\n"})
	mk("hwmon2", map[string]string{
		"name": "coretemp\n", "temp1_input": "47000\n", "temp1_crit": "100000\n", "temp1_label": "Package id 0\n",
	})
	// nvme : _max sentinelle aberrante (~65000 °C) — doit être totalement ignoré.
	mk("hwmon3", map[string]string{
		"name": "nvme\n", "temp1_input": "41850\n", "temp1_max": "65261850\n", "temp1_label": "Sensor 1\n",
	})

	got := readSensorsFromDir(root, filepath.Join(root, "nonexistent-thermal"))
	if len(got) != 1 {
		t.Fatalf("attendu 1 capteur CPU (coretemp), got %d: %+v", len(got), got)
	}
	r := got[0]
	if r.Sensor != "coretemp" || r.Temp != 47.0 || !r.HasMax || r.Max != 100.0 {
		t.Errorf("reading = %+v, want coretemp/47/Tjmax 100", r)
	}
}

func TestReadThermalZones_FiltersNonCPU(t *testing.T) {
	root := t.TempDir()
	thermal := filepath.Join(root, "thermal")
	mkzone := func(name, typ, temp string) {
		z := filepath.Join(thermal, name)
		if err := os.MkdirAll(z, 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(z, "type"), []byte(typ+"\n"), 0o644)
		os.WriteFile(filepath.Join(z, "temp"), []byte(temp+"\n"), 0o644)
	}
	mkzone("thermal_zone0", "cpu-thermal", "52000")
	mkzone("thermal_zone1", "gpu-thermal", "60000") // doit être écarté

	got := readSensorsFromDir(filepath.Join(root, "nonexistent-hwmon"), thermal)
	if len(got) != 1 || got[0].Sensor != "cpu-thermal" {
		t.Fatalf("attendu seulement cpu-thermal, got %+v", got)
	}
}

func TestReadHwmon_RockchipSoC(t *testing.T) {
	// Reproduit un tourniquet ARM Rockchip : capteur CPU = soc_thermal, GPU
	// séparé (gpu_thermal) à écarter.
	root := t.TempDir()
	mk := func(dir string, files map[string]string) {
		d := filepath.Join(root, dir)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(d, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("hwmon0", map[string]string{"name": "soc_thermal\n", "temp1_input": "40000\n"})
	mk("hwmon1", map[string]string{"name": "gpu_thermal\n", "temp1_input": "38888\n"})

	got := readSensorsFromDir(root, filepath.Join(root, "nonexistent-thermal"))
	if len(got) != 1 || got[0].Sensor != "soc_thermal" || got[0].Temp != 40.0 {
		t.Fatalf("attendu soc_thermal/40, got %+v", got)
	}
}

func TestReadThermalZones_RockchipSoC(t *testing.T) {
	// Même carte via le fallback thermal_zone : soc-thermal gardé, gpu-thermal écarté.
	root := t.TempDir()
	thermal := filepath.Join(root, "thermal")
	mkzone := func(name, typ, temp string) {
		z := filepath.Join(thermal, name)
		if err := os.MkdirAll(z, 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(z, "type"), []byte(typ+"\n"), 0o644)
		os.WriteFile(filepath.Join(z, "temp"), []byte(temp+"\n"), 0o644)
	}
	mkzone("thermal_zone0", "soc-thermal", "40000")
	mkzone("thermal_zone1", "gpu-thermal", "38888") // doit être écarté

	got := readSensorsFromDir(filepath.Join(root, "nonexistent-hwmon"), thermal)
	if len(got) != 1 || got[0].Sensor != "soc-thermal" || got[0].Temp != 40.0 {
		t.Fatalf("attendu seulement soc-thermal/40, got %+v", got)
	}
}

func TestUpdateThermalMetrics_NoPanic(t *testing.T) {
	// Sur l'hôte de CI il n'y a pas forcément de /sys/class/hwmon ni de
	// /dev/kmsg lisible : la mise à jour ne doit jamais paniquer.
	RegisterThermalMetrics()
	UpdateThermalMetrics("ns-test", "node-test")
}

func TestReadSensorsFromDir_ThermalZoneFallback(t *testing.T) {
	root := t.TempDir()
	hwmonEmpty := filepath.Join(root, "hwmon-empty")
	thermal := filepath.Join(root, "thermal")
	zone := filepath.Join(thermal, "thermal_zone0")
	if err := os.MkdirAll(zone, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(zone, "type"), []byte("cpu-thermal\n"), 0o644)
	os.WriteFile(filepath.Join(zone, "temp"), []byte("52000\n"), 0o644)

	got := readSensorsFromDir(hwmonEmpty, thermal)
	if len(got) != 1 {
		t.Fatalf("expected 1 thermal_zone reading, got %d (%+v)", len(got), got)
	}
	r := got[0]
	if r.Sensor != "cpu-thermal" || r.Temp != 52.0 || r.HasMax {
		t.Errorf("reading = %+v, want cpu-thermal/52/no-max", r)
	}
}
