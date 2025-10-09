#!/usr/bin/env python3
"""
Fix nameKo to use proper Korean transliteration of official names
Not aliases, but actual Korean spelling of the name
"""

import json

def load_json(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        return json.load(f)

def save_json(filepath, data):
    with open(filepath, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

# Official Korean transliterations (proper spelling, not aliases)
OFFICIAL_KOREAN_NAMES = {
    # JP Gen 0
    "Tokino Sora": "토키노 소라",
    "Roboco-san": "로보코상",
    "Sakura Miko": "사쿠라 미코",
    "Hoshimachi Suisei": "호시마치 스이세이",
    "AZKi": "아즈키",

    # JP Gen 1
    "Yozora Mel": "요조라 멜",
    "Akai Haato": "아카이 하아토",
    "Aki Rosenthal": "아키 로젠탈",
    "Natsuiro Matsuri": "나츠이로 마츠리",
    "Shirakami Fubuki": "시라카미 후부키",

    # JP Gen 2
    "Minato Aqua": "미나토 아쿠아",
    "Murasaki Shion": "무라사키 시온",
    "Nakiri Ayame": "나키리 아야메",
    "Yuzuki Choco": "유즈키 초코",
    "Oozora Subaru": "오오조라 스바루",

    # JP Gamers
    "Ookami Mio": "오오카미 미오",
    "Nekomata Okayu": "네코마타 오카유",
    "Inugami Korone": "이누가미 코로네",

    # JP Gen 3
    "Usada Pekora": "우사다 페코라",
    "Uruha Rushia": "우루하 루시아",
    "Shiranui Flare": "시라누이 후레아",
    "Shirogane Noel": "시로가네 노엘",
    "Houshou Marine": "호쇼 마린",

    # JP Gen 4
    "Amane Kanata": "아마네 카나타",
    "Tsunomaki Watame": "츠노마키 와타메",
    "Tokoyami Towa": "토코야미 토와",
    "Himemori Luna": "히메모리 루나",

    # JP Gen 5
    "Yukihana Lamy": "유키하나 라미",
    "Momosuzu Nene": "모모스즈 네네",
    "Shishiro Botan": "시시로 보탄",
    "Omaru Polka": "오마루 폴카",

    # JP Gen 6 (holoX)
    "La+ Darknesss": "라플라스 다크니스",
    "Takane Lui": "타카네 루이",
    "Hakui Koyori": "하쿠이 코요리",
    "Kazama Iroha": "카자마 이로하",
    "Sakamata Chloe": "사카마타 클로에",

    # JP ReGLOSS
    "Hiodoshi Ao": "히오도시 아오",
    "Otonose Kanade": "오토노세 카나데",
    "Ichijou Ririka": "이치죠 리리카",
    "Juufuutei Raden": "주후테이 라덴",
    "Todoroki Hajime": "토도로키 하지메",

    # JP FLOW GLOW
    "Isaki Riona": "이사키 리오나",
    "Koganei Niko": "코가네이 니코",
    "Mizumiya Su": "미즈미야 스우",
    "Rindo Chihaya": "린도 치하야",
    "Kikirara Vivi": "키키라라 비비",

    # ID Gen 1
    "Ayunda Risu": "아윤다 리스",
    "Moona Hoshinova": "무나 호시노바",
    "Airani Iofifteen": "아이라니 이오피프틴",

    # ID Gen 2
    "Kureiji Ollie": "쿠레이지 올리",
    "Anya Melfissa": "아냐 멜피사",
    "Pavolia Reine": "파볼리아 레이네",

    # ID Gen 3
    "Vestia Zeta": "베스티아 제타",
    "Kaela Kovalskia": "카엘라 코발스키아",
    "Kobo Kanaeru": "코보 카나에루",

    # EN Myth
    "Mori Calliope": "모리 칼리오페",
    "Takanashi Kiara": "타카나시 키아라",
    "Ninomae Ina'nis": "니노마에 이나니스",
    "Gawr Gura": "가우르 구라",
    "Watson Amelia": "왓슨 아멜리아",

    # EN Promise (Council)
    "Tsukumo Sana": "츠쿠모 사나",
    "Ceres Fauna": "세레스 파우나",
    "Ouro Kronii": "오로 크로니",
    "Nanashi Mumei": "나나시 무메이",
    "Hakos Baelz": "하코스 벨즈",

    # EN Advent
    "Shiori Novella": "시오리 노벨라",
    "Koseki Bijou": "코세키 비쥬",
    "Nerissa Ravencroft": "네리사 레이븐크로프트",
    "FuwaMoco": "후와모코",

    # EN Justice
    "Elizabeth Rose Bloodflame": "엘리자베스 로즈 블러드플레임",
    "Cecilia Immergreen": "세실리아 이머그린",
    "Raora Panthera": "라오라 판테라",
    "Gigi Murin": "지지 무린",
}

def main():
    print("Loading members.json...")
    members_data = load_json('internal/domain/data/members.json')

    updated = 0
    missing = []

    for member in members_data['members']:
        name = member['name']

        if name in OFFICIAL_KOREAN_NAMES:
            new_ko = OFFICIAL_KOREAN_NAMES[name]
            old_ko = member.get('nameKo', '')

            if old_ko != new_ko:
                member['nameKo'] = new_ko
                print(f"✅ {name}")
                print(f"   Old: {old_ko}")
                print(f"   New: {new_ko}")
                updated += 1
        else:
            missing.append(name)
            # Keep original
            if 'nameKo' not in member or not member['nameKo']:
                member['nameKo'] = name

    print(f"\n=== Summary ===")
    print(f"Total members: {len(members_data['members'])}")
    print(f"Updated: {updated}")
    print(f"Missing mapping: {len(missing)}")

    if missing:
        print(f"\nMembers without Korean mapping:")
        for name in missing:
            print(f"  - {name}")

    # Save backup
    save_json('internal/domain/data/members.json.backup3', members_data)
    print(f"\n✅ Backup: members.json.backup3")

    # Save updated
    save_json('internal/domain/data/members.json', members_data)
    print(f"✅ Updated: members.json")

if __name__ == "__main__":
    main()
