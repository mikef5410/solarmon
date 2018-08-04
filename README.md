# Solarmon monitors my electric grid usage and solar generation

This code is a daemon written in Go to extract data from my solar inverter (a SolarEdge) and from my smart meter (Pacfic Gas & Electric), as gatewayed by a Rainforest Eagle-200 box. PG&E will allow multiple devices in the home to hook to the meter via Zigbee.

What I wanted as a close to realtime view of house demand, grid demand, and solar generation with accumulators for energy use and generation.

The code uses the Rainforest local API, which returns an XML with very little data: 
 * The total number of kWh pulled from the grid
 * The total number of kWk pushed into the grid
 * Current instantaneous demand in watts
 
The SolarEdge gives me somewhat more information, but per-panel performance comes via a proprietary, opaque protocol, sadly. Inverter info is available via MODBUS over TCP, or MODBUS via RS485.

The code starts two output goroutines, one to write a simple YAML file for live data output, and one to write logged data into an sqlite3 database. Then, it loops infinitely, starting two one-shot goroutines to acquire the data. Once we get results back from both, it's digested and shipped to the output goroutines to write a new live data file, and insert into the db.

Presentation is via the web. The main web page serves a bit of javascript that loads the latest live data YAML, parses it and presents it.
