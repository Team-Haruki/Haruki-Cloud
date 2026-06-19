package accountdata

import (
	"context"
	"fmt"
	"sort"

	pjskdb "haruki-cloud/database/pjsk"
	"haruki-cloud/database/pjsk/userbinding"
)

func effectiveBindingDisplayOrder(binding *pjskdb.UserBinding) int {
	if binding == nil {
		return 0
	}
	if binding.DisplayOrder > 0 {
		return binding.DisplayOrder
	}
	if binding.ID > 0 {
		return binding.ID
	}
	return 0
}

func effectiveBindingListDisplayOrder(item BindingListItem) int {
	if item.DisplayOrder > 0 {
		return item.DisplayOrder
	}
	if item.BindingID > 0 {
		return item.BindingID
	}
	return 0
}

func nextBindingDisplayOrderTx(ctx context.Context, tx *pjskdb.Tx, harukiUserID int) (int, error) {
	bindings, err := tx.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		WithGameAccount().
		All(ctx)
	if err != nil {
		return 0, err
	}

	maxOrder := 0
	for _, binding := range bindings {
		if order := effectiveBindingDisplayOrder(binding); order > maxOrder {
			maxOrder = order
		}
	}
	return maxOrder + 1, nil
}

func ensureBindingDisplayOrdersTx(ctx context.Context, tx *pjskdb.Tx, harukiUserID int) error {
	bindings, err := tx.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		WithGameAccount().
		All(ctx)
	if err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}

	seen := make(map[int]struct{}, len(bindings))
	needsNormalize := false
	for _, binding := range bindings {
		if binding.DisplayOrder <= 0 {
			needsNormalize = true
			break
		}
		if _, ok := seen[binding.DisplayOrder]; ok {
			needsNormalize = true
			break
		}
		seen[binding.DisplayOrder] = struct{}{}
	}
	if !needsNormalize {
		return nil
	}

	sort.Slice(bindings, func(i, j int) bool {
		orderI := effectiveBindingDisplayOrder(bindings[i])
		orderJ := effectiveBindingDisplayOrder(bindings[j])
		if orderI != orderJ {
			return orderI < orderJ
		}
		return bindings[i].ID < bindings[j].ID
	})

	for idx, binding := range bindings {
		displayOrder := idx + 1
		if binding.DisplayOrder == displayOrder {
			continue
		}
		if _, err := tx.UserBinding.UpdateOneID(binding.ID).
			SetDisplayOrder(displayOrder).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *BindingService) Swap(ctx context.Context, platform, platformUserID, leftSelector, rightSelector, server string) ([]BindingListItem, error) {
	if err := s.requireReady(platform, platformUserID); err != nil {
		return nil, err
	}
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	leftSelector = normalizeUID(leftSelector)
	rightSelector = normalizeUID(rightSelector)
	if leftSelector == "" || rightSelector == "" {
		return nil, fmt.Errorf("请提供两个要交换的账号序号，例如 /绑定交换 u1 u2")
	}
	if leftSelector == rightSelector {
		return nil, fmt.Errorf("请提供两个不同的账号序号")
	}

	harukiUserID, err := s.identity.ResolveOrCreate(ctx, platform, platformUserID)
	if err != nil {
		return nil, err
	}

	tx, err := s.pjskDB.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := ensureBindingDisplayOrdersTx(ctx, tx, harukiUserID); err != nil {
		return nil, err
	}

	bindings, err := tx.UserBinding.Query().
		Where(userbinding.HarukiUserID(harukiUserID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := buildBindingList(bindings, nil)
	leftItem, err := selectBinding(items, leftSelector, server)
	if err != nil {
		return nil, err
	}
	rightItem, err := selectBinding(items, rightSelector, server)
	if err != nil {
		return nil, err
	}
	if leftItem.BindingID == rightItem.BindingID {
		return nil, fmt.Errorf("请提供两个不同的账号序号")
	}

	if _, err := tx.UserBinding.UpdateOneID(leftItem.BindingID).
		SetDisplayOrder(effectiveBindingListDisplayOrder(rightItem)).
		Save(ctx); err != nil {
		return nil, err
	}
	if _, err := tx.UserBinding.UpdateOneID(rightItem.BindingID).
		SetDisplayOrder(effectiveBindingListDisplayOrder(leftItem)).
		Save(ctx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil

	return s.List(ctx, platform, platformUserID)
}
