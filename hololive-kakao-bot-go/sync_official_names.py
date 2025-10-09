#!/usr/bin/env python3
"""
Sync official names from hololive.hololivepro.com/talents to members.json
Updates:
1. nameJa: Official Japanese name from talents page
2. aliases.ja: Add schedule scraping names
3. nameKo: Use first Korean alias as official Korean name
"""

import json
import sys

def load_json(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def save_json(filepath, data):
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

def normalize_english_name(name):
    """Normalize English name for matching"""
    # Handle variations
    replacements = {
        "Robocosan": "Roboco-san",
    }
    return replacements.get(name, name)

def main():
    print("Loading data files...")
    official_talents = load_json('official_talents.json')
    schedule_names = load_json('official_japanese_names.json')
    members_data = load_json('internal/domain/data/members.json')

    # Build lookup maps
    official_map = {}  # English name -> Japanese official name
    for talent in official_talents:
        en_name = normalize_english_name(talent['english'])
        official_map[en_name] = talent['japanese']

    schedule_ja_names = {entry['member_name'] for entry in schedule_names
                         if entry['member_name'] not in ["カルーセル", "ホロライブ", "ReGLOSS", "FLOW GLOW"]}

    print(f"Official talents: {len(official_map)}")
    print(f"Schedule Japanese names: {len(schedule_ja_names)}\n")

    # Update members
    updated_count = 0
    nameJa_updated = 0
    nameKo_added = 0
    ja_alias_added = 0

    for member in members_data['members']:
        member_name = member['name']
        updated = False

        # 1. Update nameJa with official Japanese name
        if member_name in official_map:
            official_ja = official_map[member_name]

            # Update if different or missing
            if member.get('nameJa') != official_ja:
                old_name = member.get('nameJa', '(none)')
                member['nameJa'] = official_ja
                print(f"✅ Updated nameJa for {member_name}:")
                print(f"   Old: {old_name}")
                print(f"   New: {official_ja}")
                nameJa_updated += 1
                updated = True

        # 2. Add nameKo if missing (use first Korean alias)
        if 'nameKo' not in member or not member.get('nameKo'):
            if member.get('aliases') and member['aliases'].get('ko') and len(member['aliases']['ko']) > 0:
                member['nameKo'] = member['aliases']['ko'][0]
            else:
                # Fallback: use English name
                member['nameKo'] = member['name']

            nameKo_added += 1
            updated = True

        # 3. Add schedule scraping names to aliases.ja
        member_ja = member.get('nameJa', '')

        if member.get('aliases') is None:
            member['aliases'] = {}

        if 'ja' not in member['aliases']:
            member['aliases']['ja'] = []

        ja_aliases = member['aliases']['ja']

        # Check if we should add schedule scraping names
        for schedule_name in schedule_ja_names:
            # Match if schedule name is contained in member's Japanese name or vice versa
            if (schedule_name in member_ja or
                member_ja in schedule_name or
                any(schedule_name in alias or alias in schedule_name for alias in ja_aliases)):

                # Add if not already present
                if schedule_name not in ja_aliases and schedule_name != member_ja:
                    member['aliases']['ja'].append(schedule_name)
                    print(f"  + Added '{schedule_name}' to {member_name} aliases")
                    ja_alias_added += 1
                    updated = True

        if updated:
            updated_count += 1

    # Summary
    print(f"\n=== Update Summary ===")
    print(f"Total members: {len(members_data['members'])}")
    print(f"Members updated: {updated_count}")
    print(f"  nameJa updated: {nameJa_updated}")
    print(f"  nameKo added: {nameKo_added}")
    print(f"  Japanese aliases added: {ja_alias_added}")

    # Save backup
    save_json('internal/domain/data/members.json.backup2', members_data)
    print(f"\n✅ Backup saved: members.json.backup2")

    # Save updated
    save_json('internal/domain/data/members.json', members_data)
    print(f"✅ Updated: members.json")

    print(f"\n🎯 All official names synced!")

if __name__ == "__main__":
    main()
