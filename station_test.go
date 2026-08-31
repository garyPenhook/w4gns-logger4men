package main

import (
	"path/filepath"
	"testing"
)

func TestSaveStationProfileValidatesAndPersistsStationData(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()

	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatalf("activeStationProfile returned error: %v", err)
	}
	profile.Name = "Home"
	profile.Callsign = "w4gns"
	profile.OperatorName = "Operator"
	profile.MyGridSquare = "fn31pr"
	profile.Timezone = "America/New_York"
	profile.Rig = "HF Rig"
	profile.PowerWatts = "100"
	saved, err := st.saveStationProfile(profile)
	if err != nil {
		t.Fatalf("saveStationProfile returned error: %v", err)
	}
	if saved.Callsign != "W4GNS" || saved.MyGridSquare != "FN31PR" || saved.PowerWatts != "100" {
		t.Errorf("saved profile = %#v", saved)
	}
	if saved.Latitude == nil || saved.Longitude == nil {
		t.Fatal("saved profile did not derive grid-square coordinates")
	}

	reloaded, err := st.activeStationProfile()
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if reloaded.Name != "Home" || reloaded.Callsign != "W4GNS" || reloaded.MyGridSquare != "FN31PR" || reloaded.PowerWatts != "100" {
		t.Errorf("reloaded profile = %#v", reloaded)
	}
}

func TestSaveStationProfileRejectsInvalidSettings(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatalf("activeStationProfile returned error: %v", err)
	}
	profile.Timezone = "Local"
	if _, err := st.saveStationProfile(profile); err == nil {
		t.Fatal("saveStationProfile accepted invalid timezone")
	}
	profile.Timezone = "UTC"
	profile.MyGridSquare = "ZZ99"
	if _, err := st.saveStationProfile(profile); err == nil {
		t.Fatal("saveStationProfile accepted invalid grid square")
	}
	profile.MyGridSquare = ""
	profile.PowerWatts = "many"
	if _, err := st.saveStationProfile(profile); err == nil {
		t.Fatal("saveStationProfile accepted invalid power")
	}
}
