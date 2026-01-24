#!/usr/bin/env python3
"""
Update Homebrew formula with new version and SHA256 checksums.
Usage: python3 update-homebrew-formula.py <version> <checksums_file> <formula_file>
"""
import sys
import re
import os

def extract_checksums(checksums_file):
    """Extract checksums from checksums.txt file."""
    checksums = {}
    with open(checksums_file, 'r') as f:
        for line in f:
            parts = line.strip().split()
            if len(parts) >= 2:
                sha256 = parts[0]
                filename = parts[1]
                checksums[filename] = sha256
    return checksums

def update_formula(formula_file, version, checksums):
    """Update formula file with new version and checksums."""
    with open(formula_file, 'r') as f:
        lines = f.readlines()
    
    # Strip 'v' prefix from version if present (version property must not have 'v' prefix)
    version_no_v = version[1:] if version.startswith('v') else version
    
    # Ensure 'v' prefix for URLs
    version_url = version if version.startswith('v') else f'v{version}'
    
    # Map binary names to their checksums
    checksum_map = {
        'mdserver-macos-arm64': checksums.get('mdserver-macos-arm64', ''),
        'mdserver-macos-amd64': checksums.get('mdserver-macos-amd64', ''),
        'mdserver-linux-arm64': checksums.get('mdserver-linux-arm64', ''),
        'mdserver-linux-amd64': checksums.get('mdserver-linux-amd64', ''),
    }
    
    updated_lines = []
    i = 0
    while i < len(lines):
        line = lines[i]
        
        # Update version line
        if re.match(r'\s*version\s+"[^"]*"', line):
            updated_lines.append(f'  version "{version_no_v}"\n')
            i += 1
            # Skip revision line if present (new version resets revisions)
            if i < len(lines) and re.match(r'\s*revision\s+\d+', lines[i]):
                i += 1  # Skip the revision line
            continue
        
        # Remove revision lines (revisions are reset when version changes)
        if re.match(r'\s*revision\s+\d+', line):
            i += 1
            continue
        
        # Update URL lines with new version
        url_match = re.search(r'releases/download/(v?[^/]+)/(mdserver-[^"]+)', line)
        if url_match:
            binary_name = url_match.group(2)
            # Replace version in URL
            new_line = re.sub(
                r'releases/download/[^/]+/',
                f'releases/download/{version_url}/',
                line
            )
            updated_lines.append(new_line)
            i += 1
            
            # Check if next line is sha256 and update it
            if i < len(lines) and 'sha256' in lines[i]:
                sha256 = checksum_map.get(binary_name, '')
                if sha256:
                    # Replace the sha256 value
                    sha256_line = re.sub(r'sha256\s+"[^"]*"', f'sha256 "{sha256}"', lines[i])
                    updated_lines.append(sha256_line)
                else:
                    updated_lines.append(lines[i])
                i += 1
            continue
        
        updated_lines.append(line)
        i += 1
    
    with open(formula_file, 'w') as f:
        f.writelines(updated_lines)
    
    print(f"✓ Updated formula with version {version}")
    print("  Removed revision (new version resets revisions)")
    for binary_name, sha256 in checksum_map.items():
        if sha256:
            print(f"  {binary_name}: {sha256}")

def main():
    if len(sys.argv) != 4:
        print("Usage: python3 update-homebrew-formula.py <version> <checksums_file> <formula_file>")
        sys.exit(1)
    
    version = sys.argv[1]
    checksums_file = sys.argv[2]
    formula_file = sys.argv[3]
    
    if not os.path.exists(checksums_file):
        print(f"Error: Checksums file not found: {checksums_file}")
        sys.exit(1)
    
    if not os.path.exists(formula_file):
        print(f"Error: Formula file not found: {formula_file}")
        sys.exit(1)
    
    checksums = extract_checksums(checksums_file)
    
    # Verify we have all required checksums
    required = ['mdserver-macos-arm64', 'mdserver-macos-amd64', 
                'mdserver-linux-arm64', 'mdserver-linux-amd64']
    missing = [r for r in required if r not in checksums]
    if missing:
        print(f"Error: Missing checksums for: {', '.join(missing)}")
        sys.exit(1)
    
    update_formula(formula_file, version, checksums)

if __name__ == '__main__':
    main()
