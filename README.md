# S3 Bucket Hunter

![Go](https://img.shields.io/badge/Go-1.20+-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-MIT-green)
![Platform](https://img.shields.io/badge/Platform-Linux-blue)

A lightweight, high-performance Go toolset for identifying publicly accessible Amazon S3 buckets and interactively browsing their contents — for **authorized security assessments, bug bounty engagements, and educational research** — all without requiring AWS credentials.

---

## 🛠️ System Components

The suite is divided into two decoupled Go applications that interact via a shared flat-file registry:

| Component | File | Description |
|---|---|---|
| **Detector** | `detector.go` | Scans a target list of URLs/endpoints, identifies open or misconfigured S3 buckets, and registers them. |
| **Manager** | `manager.go` | An interactive TUI terminal application that parses discovered buckets, allowing you to browse, download, upload, or delete remote data. |

## Repository Structure

```text
.
├── detector.go
├── manager.go
├── aws_s3_endpoints.txt
├── aws_final.txt
├── public_buckets.txt
├── README.md
└── LICENSE
```

---

## 🚀 Getting Started

### Prerequisites

- Go 1.20+ (1.16+ minimum)
- AWS CLI v2 installed and accessible in your system's `$PATH` (used under the hood by the Manager utility)
- Linux (Kali, Ubuntu, Debian, Parrot)

### Installation & Compilation

```bash
git clone https://github.com/DashratNandaniya/s3-bucket-hunter.git
cd s3-bucket-hunter

# Compile the Detector tool
go build -o detector detector.go

# Compile the Manager tool
go build -o manager manager.go
```

---

## 📖 Usage Guide

The tools are designed to work sequentially: build a target list, scan it with the Detector, then explore results with the Manager.

### Step 1: Build a Target List from `aws_s3_endpoints.txt`

The Detector expects `targets.txt` to contain one fully-qualified bucket hostname per line:

```
my-test-bucket.s3.amazonaws.com
another-exposed-bucket.s3.us-east-1.amazonaws.com
```

Since every bucket's virtual-hosted-style URL is technically a subdomain of one of AWS's S3 endpoints, you can generate candidates by running subdomain enumeration tools directly against each entry in `aws_s3_endpoints.txt`, treating each endpoint as the "domain" to enumerate against.

1. **Run subdomain enumeration tools against each endpoint** in the list, feeding them in one at a time (or in bulk, if your tool supports a domain-list input):

   ```bash
   # subfinder — bulk mode using the endpoints file directly
   subfinder -dL aws_s3_endpoints.txt -o subfinder_out.txt

   # amass — loop over each endpoint (passive mode)
   while read -r endpoint; do
     amass enum -passive -d "$endpoint"
   done < aws_s3_endpoints.txt >> amass_out.txt

   # assetfinder — loop over each endpoint
   while read -r endpoint; do
     assetfinder --subs-only "$endpoint"
   done < aws_s3_endpoints.txt >> assetfinder_out.txt
   ```

2. **Merge the output** from all tools into a single candidate list:

   ```bash
   cat subfinder_out.txt amass_out.txt assetfinder_out.txt > all_candidates.txt
   ```

3. **Sort and dedupe** the merged candidate list into your final target file:

   ```bash
   sort -u all_candidates.txt > targets.txt
   ```

> 💡 Tip: Running the same endpoint through several tools in parallel (step 1) and merging results (step 2) turns up more candidates than any single tool alone, since each draws on different data sources (certificate transparency logs, passive DNS, search engines, etc.).

### Step 2: Scan and Detect Public Buckets

The detector reads your `targets.txt` file, validates each candidate using asynchronous HTTP requests, and saves misconfigured public listings to an output file.

Execute the compiled binary:

```bash
./detector
```

When prompted, type the name of your target file (e.g., `targets.txt`).

Any discovered public buckets will automatically be recorded into a newly generated `public_buckets.txt` registry file.

### Step 3: Explore and Modify Storage via the Manager

Once you have populated `public_buckets.txt`, launch the interactive manager interface:

```bash
./manager
```

Select the index number of the public bucket you want to navigate from the 3-column split grid, then use the integrated command system to interact with the file tree.

---

## 🕹️ Manager TUI Command Matrix

Once you "travel" inside a bucket, you can manage objects using the following low-overhead, single-character execution menu:

```
o <n>:enter      d <n>:download   f:dl-folder      u [path]:upload
r <n>:delete     b:back           l:refresh        q:exit
```

| Command | Name | Description |
|---|---|---|
| `o <number>` | Open/Enter | If the selection is a folder, it drills into that path. If it's a file, it pipes the remote content directly to your terminal standard output (stdout). |
| `d <number>` | Download Specific | Downloads a single file or directory to a chosen local path context. |
| `f` | Download Entire Folder | Recursively downloads everything inside your current remote browsing directory down to your local machine. |
| `u [optional path]` | Upload Data | Uploads a local file or directory straight into the current S3 prefix path. |
| `r <number>` | Remove/Delete | Permanently deletes an object or folder hierarchy from the target bucket instance. |
| `b` | Back | Steps out one level to the parent directory path context. |
| `l` | Refresh | Rebuilds the recursive bucket index directly from AWS. |
| `q` | Exit | Quits the current bucket workspace view and returns you to the target selection landing grid. |

---

## ⚠️ Built-in Security Guards

- **Host System Protection Loop Guard** — The manager explicitly blocks running an upload command targeted at `/` or `/*`. This prevents accidental catastrophic recursive synchronization loops from uploading your entire host operating system context to the cloud.
- **Smart Elevation Checks** — Before executing any downloads, the system runs local write permission validation tests. If a location requires root privileges, it gracefully and dynamically invokes a secure `sudo` prompt interface.
- **Protected Read-Only Grace** — If an S3 bucket is globally visible but blocks write permissions, attempting an upload (`u`) or deletion (`r`) will fail safely without breaking execution flow, displaying a clean target warning alert.

---

## Output Files

| File | Purpose |
|---|---|
| `aws_s3_endpoints.txt` | Seed list of AWS S3 regional endpoints |
| `targets.txt` | Enumeration results / scan candidates |
| `public_buckets.txt` | Confirmed public buckets |
| `aws_final.txt` | Final processed list |

---

## Roadmap

- [ ] Concurrent scanning
- [ ] JSON output
- [ ] CSV export
- [ ] HTML reports

## Contributing

Pull requests and feature suggestions are welcome.

---

## ⚖️ Legal Disclaimer & Responsible Use

This project is intended **strictly** for authorized security research, bug bounty engagements, and educational purposes. By using this tool, you agree to the following:

- **Get authorization first.** Only run this toolset against S3 buckets, domains, or infrastructure you own, or for which you have explicit written permission to test (e.g., a signed engagement letter, an active bug bounty program scope, or your own organization's assets).
- **No unauthorized access.** Scanning, downloading, uploading to, or deleting data from a bucket you don't have permission to test may violate laws such as the U.S. Computer Fraud and Abuse Act (CFAA), the UK Computer Misuse Act, the EU's GDPR/NIS2 framework, or equivalent legislation in your jurisdiction — regardless of whether the bucket is publicly accessible.
- **Handle discovered data responsibly.** If you find exposed sensitive data during authorized testing, follow responsible disclosure practices: report it to the asset owner or the relevant bug bounty program, avoid exfiltrating more data than necessary to prove impact, and never publish, sell, or redistribute discovered data.
- **No warranty.** This software is provided "as is" with no guarantee of fitness for any particular purpose. The authors are not responsible for misuse, damages, or legal consequences resulting from use of this tool.
- **You are responsible for your own actions.** The maintainers of this repository do not condone and are not liable for any illegal or unethical use of this software.

If you're unsure whether you have authorization to test a target, don't — get explicit written permission first.

## 📄 License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for full terms. In short: you're free to use, modify, and distribute this software, provided the original copyright notice is retained.
