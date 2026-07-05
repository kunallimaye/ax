// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/adk/v2/agent"
)

// weatherInput is the get_weather tool's argument schema (inferred by ADK from
// the struct + json tags).
type weatherInput struct {
	City string `json:"city" description:"The city to get the weather for, e.g. 'London'."`
}

// weatherOutput is the get_weather tool's result schema.
type weatherOutput struct {
	City         string  `json:"city"`
	TemperatureC float64 `json:"temperature_c"`
	WindKph      float64 `json:"wind_kph"`
	Conditions   string  `json:"conditions"`
	Summary      string  `json:"summary"`
}

// getWeather is the tool handler. It uses the keyless Open-Meteo APIs (geocoding
// + current forecast) so it returns live data with no API key or secret.
//
// ADK v2 unified ToolContext/CallbackContext into a single agent.Context.
func getWeather(ctx agent.Context, in weatherInput) (weatherOutput, error) {
	lat, lon, resolved, err := geocode(ctx, in.City)
	if err != nil {
		return weatherOutput{}, err
	}
	tempC, windKph, code, err := currentWeather(ctx, lat, lon)
	if err != nil {
		return weatherOutput{}, err
	}
	conditions := weatherCodeText(code)
	return weatherOutput{
		City:         resolved,
		TemperatureC: tempC,
		WindKph:      windKph,
		Conditions:   conditions,
		Summary: fmt.Sprintf("%s: %s, %.0f°C, wind %.0f km/h",
			resolved, conditions, tempC, windKph),
	}, nil
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func geocode(ctx context.Context, city string) (lat, lon float64, name string, err error) {
	u := "https://geocoding-api.open-meteo.com/v1/search?count=1&language=en&format=json&name=" + url.QueryEscape(city)
	var out struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Name      string  `json:"name"`
			Country   string  `json:"country"`
		} `json:"results"`
	}
	if err := getJSON(ctx, u, &out); err != nil {
		return 0, 0, "", fmt.Errorf("geocoding %q: %w", city, err)
	}
	if len(out.Results) == 0 {
		return 0, 0, "", fmt.Errorf("no location found for %q", city)
	}
	r := out.Results[0]
	name = r.Name
	if r.Country != "" {
		name = r.Name + ", " + r.Country
	}
	return r.Latitude, r.Longitude, name, nil
}

func currentWeather(ctx context.Context, lat, lon float64) (tempC, windKph float64, code int, err error) {
	u := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&current=temperature_2m,wind_speed_10m,weather_code", lat, lon)
	var out struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			WindSpeed   float64 `json:"wind_speed_10m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
	}
	if err := getJSON(ctx, u, &out); err != nil {
		return 0, 0, 0, fmt.Errorf("forecast: %w", err)
	}
	return out.Current.Temperature, out.Current.WindSpeed, out.Current.WeatherCode, nil
}

func getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, u)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// weatherCodeText maps WMO weather codes to a short description.
func weatherCodeText(code int) string {
	switch code {
	case 0:
		return "clear sky"
	case 1, 2, 3:
		return "partly cloudy"
	case 45, 48:
		return "foggy"
	case 51, 53, 55, 56, 57:
		return "drizzle"
	case 61, 63, 65, 66, 67:
		return "rain"
	case 71, 73, 75, 77:
		return "snow"
	case 80, 81, 82:
		return "rain showers"
	case 85, 86:
		return "snow showers"
	case 95, 96, 99:
		return "thunderstorm"
	default:
		return "unknown conditions"
	}
}
