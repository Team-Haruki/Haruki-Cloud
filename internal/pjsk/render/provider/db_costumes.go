package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"entgo.io/ent/dialect/sql"

	sekaiDB "haruki-cloud/database/sekai"
	"haruki-cloud/database/sekai/cardcostume3d"
	"haruki-cloud/database/sekai/costume3d"
	"haruki-cloud/database/sekai/predicate"
	renderregion "haruki-cloud/internal/pjsk/region"
	"haruki-cloud/internal/pjsk/render/common"
	"haruki-cloud/internal/pjsk/render/masterdata"
)

type dbCostumeProvider struct {
	client *sekaiDB.Client
	region renderregion.Value
}

func (p *dbCostumeProvider) GetByID(ctx context.Context, id int) (*masterdata.Costume3d, error) {
	if id == 0 {
		return nil, fmt.Errorf("costume id is required")
	}
	entity, err := p.client.Costume3D.Query().
		Where(costume3d.ServerRegionEQ(p.region.String()), costume3d.GameIDEQ(int64(id))).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("query costume %d: %w", id, err)
	}
	return common.CloneCostume(common.ConvertCostumeEntity(entity)), nil
}

func (p *dbCostumeProvider) Filter(ctx context.Context, filter *CostumeFilter) ([]*masterdata.Costume3d, error) {
	if filter == nil {
		filter = &CostumeFilter{}
	}
	query := p.client.Costume3D.Query().Where(costume3d.ServerRegionEQ(p.region.String()))
	query = applyDBCostumeFilter(query, filter)
	query = query.Order(
		costume3d.ByPublishedAt(sql.OrderDesc()),
		costume3d.ByArchivePublishedAt(sql.OrderDesc()),
		costume3d.BySeq(sql.OrderDesc()),
		costume3d.ByGameID(sql.OrderDesc()),
	)
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	entities, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query costumes: %w", err)
	}
	return cloneCostumeEntities(entities), nil
}

func (p *dbCostumeProvider) GetVariants(ctx context.Context, groupID int, partType string, characterID int) ([]*masterdata.Costume3d, error) {
	if groupID == 0 {
		return nil, fmt.Errorf("costume group id is required")
	}
	predicates := []predicate.Costume3D{
		costume3d.ServerRegionEQ(p.region.String()),
		costume3d.Costume3DGroupIDEQ(int64(groupID)),
	}
	if partType = strings.TrimSpace(partType); partType != "" {
		predicates = append(predicates, costume3d.PartTypeEQ(partType))
	}
	if characterID > 0 {
		predicates = append(predicates, costume3d.CharacterIDEQ(int64(characterID)))
	}
	entities, err := p.client.Costume3D.Query().
		Where(predicates...).
		Order(costume3d.ByColorID(), costume3d.ByGameID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query costume variants for group %d: %w", groupID, err)
	}
	return cloneCostumeEntities(entities), nil
}

func (p *dbCostumeProvider) GetSourceCardIDs(ctx context.Context, costumeIDs []int) (map[int][]int, error) {
	if len(costumeIDs) == 0 {
		return map[int][]int{}, nil
	}
	gameIDs := make([]int64, 0, len(costumeIDs))
	for _, id := range costumeIDs {
		if id > 0 {
			gameIDs = append(gameIDs, int64(id))
		}
	}
	if len(gameIDs) == 0 {
		return map[int][]int{}, nil
	}
	links, err := p.client.Cardcostume3D.Query().
		Where(cardcostume3d.ServerRegionEQ(p.region.String()), cardcostume3d.Costume3DIDIn(gameIDs...)).
		Order(cardcostume3d.ByCostume3DID(), cardcostume3d.ByCardID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query costume source cards: %w", err)
	}
	result := make(map[int][]int, len(gameIDs))
	for _, link := range links {
		costumeID := int(link.Costume3DID)
		cardID := int(link.CardID)
		if costumeID <= 0 || cardID <= 0 {
			continue
		}
		result[costumeID] = append(result[costumeID], cardID)
	}
	for costumeID := range result {
		sort.Ints(result[costumeID])
	}
	return result, nil
}

func applyDBCostumeFilter(query *sekaiDB.Costume3DQuery, filter *CostumeFilter) *sekaiDB.Costume3DQuery {
	if partType := strings.TrimSpace(filter.PartType); partType != "" {
		query = query.Where(costume3d.PartTypeEQ(partType))
	}
	if costumeType := strings.TrimSpace(filter.CostumeType); costumeType != "" {
		query = query.Where(costume3d.Costume3DTypeEQ(costumeType))
	}
	if filter.CharacterID > 0 {
		query = query.Where(costume3d.CharacterIDEQ(int64(filter.CharacterID)))
	}
	if len(filter.CharacterIDs) > 0 {
		ids := make([]int64, 0, len(filter.CharacterIDs))
		for _, id := range filter.CharacterIDs {
			if id > 0 {
				ids = append(ids, int64(id))
			}
		}
		if len(ids) > 0 {
			query = query.Where(costume3d.CharacterIDIn(ids...))
		}
	}
	if filter.ColorID > 0 {
		query = query.Where(costume3d.ColorIDEQ(int64(filter.ColorID)))
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		query = query.Where(costume3d.Or(
			costume3d.NameContainsFold(keyword),
			costume3d.ColorNameContainsFold(keyword),
			costume3d.HowToObtainContainsFold(keyword),
			costume3d.DesignerContainsFold(keyword),
			costume3d.AssetbundleNameContainsFold(keyword),
		))
	}
	return query
}

func cloneCostumeEntities(entities []*sekaiDB.Costume3D) []*masterdata.Costume3d {
	if len(entities) == 0 {
		return nil
	}
	result := make([]*masterdata.Costume3d, 0, len(entities))
	for _, entity := range entities {
		if model := common.ConvertCostumeEntity(entity); model != nil {
			result = append(result, model)
		}
	}
	return common.CloneCostumes(result)
}
