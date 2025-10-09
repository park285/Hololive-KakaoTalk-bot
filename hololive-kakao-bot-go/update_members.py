#!/usr/bin/env python3
"""
Update members.json with:
1. Official Japanese names from scraping -> aliases.ja
2. Korean official names -> nameKo field
"""

import json
import sys

def load_json(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def save_json(filepath, data):
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

def romanize_to_korean(name):
    """Convert romanized name to Korean (basic mapping)"""
    # This is a simple mapping - can be enhanced
    mapping = {
        "Usada Pekora": "우사다 페코라",
        "Sakura Miko": "사쿠라 미코",
        "Shiranui Flare": "시라누이 후레아",
        "Tokino Sora": "토키노 소라",
        "Roboco-san": "로보코상",
        "Akai Haato": "아카이 하아토",
        "Aki Rosenthal": "아키 로젠탈",
        "Shirakami Fubuki": "시라카미 후부키",
        "Natsuiro Matsuri": "나츠이로 마츠리",
        "Yozora Mel": "요조라 멜",
        "Murasaki Shion": "무라사키 시온",
        "Nakiri Ayame": "나키리 아야메",
        "Oozora Subaru": "오오조라 스바루",
        "Minato Aqua": "미나토 아쿠아",
        "Hoshimachi Suisei": "호시마치 스이세이",
        "Tsunomaki Watame": "츠노마키 와타메",
        "Tokoyami Towa": "토코야미 토와",
        "Himemori Luna": "히메모리 루나",
        "Yukihana Lamy": "유키하나 라미",
        "Momosuzu Nene": "모모스즈 네네",
        "Shishiro Botan": "시시로 보탄",
        "Omaru Polka": "오마루 폴카",
        "La+ Darknesss": "라플라스 다크니스",
        "Takane Lui": "타카네 루이",
        "Hakui Koyori": "하쿠이 코요리",
        "Kazama Iroha": "카자마 이로하",
        "Inugami Korone": "이누가미 코로네",
        "Nekomata Okayu": "네코마타 오카유",
        "Ookami Mio": "오오카미 미오",
        "Amane Kanata": "아마네 카나타",
        "AZKi": "아즈키",
    }
    return mapping.get(name, name)  # Fallback to original

def main():
    # Load data
    print("Loading data files...")
    official_names = load_json('official_japanese_names.json')
    members_data = load_json('internal/domain/data/members.json')

    # Build lookup: member name -> official japanese name
    official_ja_map = {}
    for entry in official_names:
        ja_name = entry['member_name']
        # Skip invalid entries like "カルーセル" (carousel)
        if ja_name in ["カルーセル", "ホロライブ", "ReGLOSS", "FLOW GLOW"]:
            continue
        official_ja_map[ja_name] = True

    print(f"Found {len(official_ja_map)} valid official Japanese names\n")

    # Update members
    updated_count = 0
    nameKo_added = 0
    ja_alias_added = 0

    for member in members_data['members']:
        updated = False

        # Add nameKo if missing
        if 'nameKo' not in member or not member.get('nameKo'):
            # Try to get from Korean aliases first
            if member.get('aliases') and member['aliases'].get('ko'):
                # Use first Korean alias as official Korean name
                member['nameKo'] = member['aliases']['ko'][0]
            else:
                # Romanize English name to Korean
                member['nameKo'] = romanize_to_korean(member['name'])

            nameKo_added += 1
            updated = True

        # Check if we need to add official Japanese names to aliases
        if member.get('aliases') and member['aliases'].get('ja'):
            ja_aliases = member['aliases']['ja']

            # Check each official Japanese name
            for ja_name in official_ja_map.keys():
                # Try to match this official name to this member
                member_ja = member.get('nameJa', '')

                # If official name matches member (contains their name parts)
                if (ja_name in member_ja or member_ja in ja_name or
                    any(ja_name in alias or alias in ja_name for alias in ja_aliases)):

                    # Add if not already in aliases
                    if ja_name not in ja_aliases:
                        member['aliases']['ja'].append(ja_name)
                        print(f"✅ Added '{ja_name}' to {member['name']}")
                        ja_alias_added += 1
                        updated = True

        if updated:
            updated_count += 1

    # Save updated data
    print(f"\n=== Update Summary ===")
    print(f"Total members: {len(members_data['members'])}")
    print(f"Members updated: {updated_count}")
    print(f"nameKo added: {nameKo_added}")
    print(f"Japanese aliases added: {ja_alias_added}")

    # Backup original
    save_json('internal/domain/data/members.json.backup', members_data)
    print(f"\n✅ Backup saved: members.json.backup")

    # Save updated
    save_json('internal/domain/data/members.json', members_data)
    print(f"✅ Updated: members.json")

if __name__ == "__main__":
    main()
