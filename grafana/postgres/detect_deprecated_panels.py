import json
from pathlib import Path

# Deprecated → Recommended
DEPRECATED_PANEL_MIGRATION = {
    "graph": "timeseries",
    "singlestat": "stat",
    "stat": "stat",
    "table-old": "table",
    "worldmap": "geomap",
    "table": "table",  # may still need manual review
    "text": "text (check for HTML mode)",
}

TEXT_PANEL_WITH_HTML = ("text", "html")

def check_dashboard(file_path):
    with open(file_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    deprecated_panels = []

    panels = data.get("panels", [])
    for panel in panels:
        panel_type = panel.get("type", "").lower()
        title = panel.get("title", "Untitled")

        if panel_type in DEPRECATED_PANEL_MIGRATION:
            if panel_type == "text":
                mode = panel.get("mode") or panel.get("options", {}).get("mode")
                if mode == TEXT_PANEL_WITH_HTML[1]:
                    deprecated_panels.append((title, "Text panel using HTML (AngularJS)"))
            else:
                recommended = DEPRECATED_PANEL_MIGRATION[panel_type]
                deprecated_panels.append((title, f"Deprecated panel type: '{panel_type}' → Use '{recommended}'"))

    return deprecated_panels

def scan_dashboards(folder_path, output_path):
    folder = Path(folder_path)
    json_files = list(folder.glob("*.json"))

    lines = []

    if not json_files:
        result = "No JSON files found in the folder.\n"
        print(result)
        lines.append(result)
    else:
        for json_file in json_files:
            findings = check_dashboard(json_file)
            if findings:
                lines.append(f"\n📊 Dashboard: {json_file.name}")
                for title, issue in findings:
                    lines.append(f"  - Panel: '{title}' → {issue}")
            else:
                lines.append(f"\n✅ Dashboard: {json_file.name} has no deprecated panels.")

    # Write to file
    with open(output_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))

    # Also print to terminal
    print("\n".join(lines))

if __name__ == "__main__":
    dashboards_folder = Path(__file__).parent / "v12"
    output_file = Path(__file__).parent / "deprecated_panels_report.txt"
    scan_dashboards(dashboards_folder, output_file)
