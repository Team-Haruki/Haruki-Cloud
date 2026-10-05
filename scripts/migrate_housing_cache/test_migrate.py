import copy
import unittest
from migrate import key, migrate, ORIGINS, PREFIX

class MigrationTests(unittest.TestCase):
    def test_preserves_all_sample_metadata_and_rekeys_all_regions(self):
        path='a'*64+'/12345678-1234-1234-1234-123456789abc'
        source={'version':1,'buckets':[]}
        for region in ['jp','en','cn','tw','kr']:
            entry={'cache_key':'old','competition_id':2,'submitted_at':123,'entry_name':'work','review_count':100,'last_seen_at':2000,'thumbnail_path':ORIGINS[region]+PREFIX+path if region in ORIGINS else path}
            source['buckets'].append({'key':{'region':region,'housing_id':2},'refreshed_at':1000,'sampled_at':1000,'entries':[entry]})
        original=copy.deepcopy(source)
        migrated,changed=migrate(source)
        self.assertEqual(source,original)
        self.assertEqual(changed,3)
        self.assertEqual(migrated['version'],2)
        for before,after in zip(source['buckets'],migrated['buckets']):
            expected=copy.deepcopy(before)
            expected['entries'][0]['thumbnail_path']=path
            expected['entries'][0]['cache_key']=key(expected['entries'][0])
            self.assertEqual(after,expected)

    def test_rejects_unknown_origins_and_duplicate_uploads(self):
        for path in ['https://evil.invalid/image','https://mk-prod-tos.tos-cn-shanghai.volces.com'+PREFIX+'bad']:
            with self.assertRaises(ValueError):
                migrate({'version':1,'buckets':[{'key':{'region':'cn'},'entries':[{'thumbnail_path':path}]}]})
        with self.assertRaises(ValueError):
            migrate({'version':1,'buckets':[{'key':{'region':'jp'},'entries':[{},{}]}]})
        with self.assertRaises(ValueError):migrate({'version':2})

    def test_go_key_fixture(self):
        self.assertEqual(key({'competition_id':2,'submitted_at':123,'thumbnail_path':'hash/uuid','entry_name':'work'}),'d5bbd469bace6d1c06c38dda56146a994a5213d442d6f887c2a3ea9801feb0e6')

if __name__=='__main__':unittest.main()
